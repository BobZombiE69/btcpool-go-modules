package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/samuel/go-zookeeper/zk"
)

// Client type prefix of BTCAgent
const btcAgentClientTypePrefix = "btccom-agent/"

// NiceHash client type prefix
const niceHashClientTypePrefix = "nicehash/"

// NiceHash Ethereum Stratum Protocol The protocol type prefix of
const ethereumStratumNiceHashPrefix = "ethereumstratum/"

// The version of NiceHash Ethereum Stratum Protocol used in the response
const ethereumStratumNiceHashVersion = "EthereumStratum/1.0.0"

// sendETHProxy protocol version string sent to sserver
const ethproxyVersion = "ETHProxy/1.0.0"

// The magic number of BTCAgent's ex-message
const btcAgentExMessageMagicNumber = 0x7F

// 协议检测超时时间
const protocolDetectTimeoutSeconds = 15

// Miner name get timeout
const findWorkerNameTimeoutSeconds = 60

// The timeout time for the server to respond to messages such as subscribe and authorize
const readServerResponseTimeoutSeconds = 10

// Timeout for receiving messages in pure proxy mode
// If the message cannot be received for a long time, the peer disconnection event cannot be processed in time.
// Therefore, set the receiving timeout time, give up receiving every certain time, check the status, and restart receiving
const receiveMessageTimeoutSeconds = 15

// The number of retries when the server disconnects
const retryTimeWhenServerDown = 10

// The buffer size of the created bufio Reader
const bufioReaderBufSize = 128

// ProtocolType Proxy's protocol type
type ProtocolType uint8

const (
	// ProtocolBitcoinStratum Bitcoin Stratum Protocol
	ProtocolBitcoinStratum ProtocolType = iota
	// ProtocolEthereumStratum Ethereum Ordinary Stratum Protocol
	ProtocolEthereumStratum
	// ProtocolEthereumStratumNiceHash The Ethereum Stratum protocol proposed by NiceHash
	ProtocolEthereumStratumNiceHash
	// ProtocolEthereumProxy Ethereum Stratum protocol implemented by EthProxy software
	ProtocolEthereumProxy
	// ProtocolUnknown Unknown protocol (cannot be processed)
	ProtocolUnknown
)

// RunningStat Operating status
type RunningStat uint8

const (
	// StatRunning running
	StatRunning RunningStat = iota
	// StatStoped stopped
	StatStoped RunningStat = iota
	// StatReconnecting Reconnecting to server
	StatReconnecting RunningStat = iota
)

// AuthorizeStat Certification status
type AuthorizeStat uint8

const (
	// StatConnected connected (default state)
	StatConnected AuthorizeStat = iota
	// StatSubScribed subscribed
	StatSubScribed
	// StatAuthorized verified
	StatAuthorized
)

// StratumSession is a Stratum session that contains connection and status information to the client and to the server
type StratumSession struct {
	// session manager
	manager *StratumSessionManager

	// Stratum protocol type
	protocolType ProtocolType
	// Is it BTCAgent
	isBTCAgent bool
	// Is it a NiceHash client
	isNiceHashClient bool
	// JSON-RPC version
	jsonRPCVersion int
	// Bitcoin version mask(for AsicBoost)
	versionMask uint32

	// is it running
	runningStat RunningStat
	// Server reconnection counter
	reconnectCounter uint32
	// The lock to be added when changing runningStat and switchCoinCount
	lock sync.Mutex

	clientConn   net.Conn
	clientReader *bufio.Reader

	// Client IP address and port
	clientIPPort string

	serverConn   net.Conn
	serverReader *bufio.Reader

	// sessionID Session ID, also used as Extranonce1 when mining machine
	sessionID       uint32
	sessionIDString string

	fullWorkerName   string // full miner name
	subaccountName   string // Sub account name part
	minerNameWithDot string // Miner name part (including leading ".")

	stratumSubscribeRequest *JSONRPCRequest
	stratumAuthorizeRequest *JSONRPCRequest

	// The currency mined by the user
	miningCoin string
	// Monitored Zookeeper paths
	zkWatchPath string
	// Monitored Zookeeper events
	zkWatchEvent <-chan zk.Event
}

// NewStratumSession Create a new Stratum session
func NewStratumSession(manager *StratumSessionManager, clientConn net.Conn, sessionID uint32) (session *StratumSession) {
	session = new(StratumSession)

	session.jsonRPCVersion = 1

	session.runningStat = StatStoped
	session.manager = manager
	session.sessionID = sessionID

	session.clientConn = clientConn
	session.clientReader = bufio.NewReaderSize(clientConn, bufioReaderBufSize)

	session.clientIPPort = clientConn.RemoteAddr().String()

	switch manager.chainType {
	case ChainTypeBitcoin:
		session.sessionIDString = Uint32ToHex(session.sessionID)
	case ChainTypeDecredNormal:
		// reversed 12 bytes
		session.sessionIDString = "0000000000000000" + Uint32ToHexLE(session.sessionID)
	case ChainTypeDecredGoMiner:
		// reversed 4 bytes
		session.sessionIDString = Uint32ToHexLE(session.sessionID)
	case ChainTypeEthereum:
		// Ethereum uses 24 bit session id
		session.sessionIDString = Uint32ToHex(session.sessionID)[2:8]
	}

	if glog.V(3) {
		glog.Info("IP: ", session.clientIPPort, ", Session ID: ", session.sessionIDString)
	}
	return
}

// IsRunning Check if session is running (thread safe)
func (session *StratumSession) IsRunning() bool {
	session.lock.Lock()
	defer session.lock.Unlock()

	return session.runningStat != StatStoped
}

// setStat Set session state (thread safe)
func (session *StratumSession) setStat(stat RunningStat) {
	session.lock.Lock()
	session.runningStat = stat
	session.lock.Unlock()
}

// setStatNonLock set session state (
func (session *StratumSession) setStatNonLock(stat RunningStat) {
	session.runningStat = stat
}

// getStat Get session state (thread safe)
func (session *StratumSession) getStat() RunningStat {
	session.lock.Lock()
	defer session.lock.Unlock()

	return session.runningStat
}

// getStatNonLock Get session state (lock-free, not thread-safe, for calls inside locked functions)
func (session *StratumSession) getStatNonLock() RunningStat {
	return session.runningStat
}

// getReconnectCounter Get currency switch count (thread safe)
func (session *StratumSession) getReconnectCounter() uint32 {
	session.lock.Lock()
	defer session.lock.Unlock()

	return session.reconnectCounter
}

// Run Start a Stratum session
func (session *StratumSession) Run() {
	session.lock.Lock()

	if session.runningStat != StatStoped {
		session.lock.Unlock()
		return
	}

	session.runningStat = StatRunning
	session.lock.Unlock()

	session.protocolType = session.protocolDetect()

	// In fact, there is currently only one protocol, the Stratum protocol.
	// BTCAgent also walks the Stratum protocol before the authentication is completed
	if session.protocolType == ProtocolUnknown {
		session.Stop()
		return
	}

	session.runProxyStratum()
}

// Resume Resume a Stratum session
func (session *StratumSession) Resume(sessionData StratumSessionData, serverConn net.Conn) {
	session.lock.Lock()

	if session.runningStat != StatStoped {
		session.lock.Unlock()
		return
	}

	session.runningStat = StatRunning
	session.lock.Unlock()

	// Set default protocol
	session.protocolType = session.getDefaultStratumProtocol()

	// restore server connection
	session.serverConn = serverConn
	session.serverReader = bufio.NewReaderSize(serverConn, bufioReaderBufSize)
	stat := StatConnected

	// restore version bit
	session.versionMask = sessionData.VersionMask

	if sessionData.StratumSubscribeRequest != nil {
		_, stratumErr := session.stratumHandleRequest(sessionData.StratumSubscribeRequest, &stat)
		if stratumErr != nil {
			glog.Error("Resume session ", session.clientIPPort, " failed: ", stratumErr)
			session.Stop()
			return
		}
	}

	if sessionData.StratumAuthorizeRequest != nil {
		_, stratumErr := session.stratumHandleRequest(sessionData.StratumAuthorizeRequest, &stat)
		if stratumErr != nil {
			glog.Error("Resume session ", session.clientIPPort, " failed: ", stratumErr)
			session.Stop()
			return
		}
	}

	if stat != StatAuthorized {
		glog.Error("Resume session ", session.clientIPPort, " failed: stat should be StatAuthorized, but is ", stat)
		session.Stop()
		return
	}

	err := session.findMiningCoin(false)
	if err != nil {
		glog.Error("Resume session ", session.clientIPPort, " failed: ", err)
		session.Stop()
		return
	}

	if session.miningCoin != sessionData.MiningCoin {
		glog.Error("Resume session ", session.clientIPPort, " failed: mining coin changed: ",
			sessionData.MiningCoin, " -> ", session.miningCoin)
		session.Stop()
		return
	}

	glog.Info("Resume Session Success: ", session.clientIPPort, "; ", session.fullWorkerName, "; ", session.miningCoin)

	// Then switch to pure proxy mode
	session.proxyStratum()
}

// Stop Stop a Stratum session
func (session *StratumSession) Stop() {
	session.lock.Lock()

	if session.runningStat == StatStoped {
		session.lock.Unlock()
		return
	}

	session.runningStat = StatStoped
	session.lock.Unlock()

	if session.serverConn != nil {
		session.serverConn.Close()
	}

	if session.clientConn != nil {
		session.clientConn.Close()
	}

	session.manager.ReleaseStratumSession(session)
	session.manager = nil

	if glog.V(2) {
		glog.Info("Session Stoped: ", session.clientIPPort, "; ", session.fullWorkerName, "; ", session.miningCoin)
	}
}

func (session *StratumSession) protocolDetect() ProtocolType {
	magicNumber, err := session.peekFromClientWithTimeout(1, protocolDetectTimeoutSeconds*time.Second)

	if err != nil {
		glog.Warning("read failed: ", err)
		return ProtocolUnknown
	}

	// BTC Agent sends standard Stratum protocol JSON strings during the subscribe and authorize phases.
	// The first message received from the client must be a JSON string of the Stratum protocol.
	// The ex-message may appear only after authorize is complete.
	//
	// That is to say, on the one hand, BTC Agent can share the connection and authentication process with ordinary miners,
	// On the other hand, we cannot detect that the client is a BTC Agent at the very beginning, and we have to be ready to receive ex-message at any time.
	if magicNumber[0] != '{' {
		glog.Warning("Unknown Protocol")
		return ProtocolUnknown
	}

	if glog.V(3) {
		glog.Info("Found Stratum Protocol")
	}

	return session.getDefaultStratumProtocol()
}

func (session *StratumSession) getDefaultStratumProtocol() ProtocolType {
	switch session.manager.chainType {
	case ChainTypeBitcoin:
		fallthrough
	case ChainTypeDecredNormal:
		fallthrough
	case ChainTypeDecredGoMiner:
		// DCR uses almost the exact same protocol as Bitcoin
		return ProtocolBitcoinStratum
	case ChainTypeEthereum:
		// This is the default protocol. The protocol may change after further detection.
		// The difference between ProtocolEthereumProxy and the other two Ethereum protocols is that
		// ProtocolEthereumProxy is no "mining.subscribe" phase, so it is set as default to simplify the detection.
		return ProtocolEthereumProxy
	default:
		return ProtocolUnknown
	}
}

func (session *StratumSession) runProxyStratum() {
	var err error

	err = session.stratumFindWorkerName()

	if err != nil {
		session.Stop()
		return
	}

	err = session.findMiningCoin(session.manager.enableUserAutoReg)

	if err != nil {
		session.Stop()
		return
	}

	err = session.connectStratumServer()

	if err != nil {
		session.Stop()
		return
	}

	// Then switch to pure proxy mode
	session.proxyStratum()
}

func (session *StratumSession) parseSubscribeRequest(request *JSONRPCRequest) (result interface{}, err *StratumError) {
	// Save the original subscription request for forwarding to the Stratum server
	session.stratumSubscribeRequest = request

	// generate response
	switch session.manager.chainType {
	case ChainTypeBitcoin:
		fallthrough
	case ChainTypeDecredNormal:
		fallthrough
	case ChainTypeDecredGoMiner:
		if len(request.Params) >= 1 {
			userAgent, ok := session.stratumSubscribeRequest.Params[0].(string)
			// Determine whether it is BTCAgent
			if ok && strings.HasPrefix(strings.ToLower(userAgent), btcAgentClientTypePrefix) {
				session.isBTCAgent = true
			}
		}

		result = JSONRPCArray{JSONRPCArray{JSONRPCArray{"mining.set_difficulty", session.sessionIDString}, JSONRPCArray{"mining.notify", session.sessionIDString}}, session.sessionIDString, 8}
		return

	case ChainTypeEthereum:
		// only ProtocolEthereumStratum and ProtocolEthereumStratumNiceHash has the "mining.subscribe" phase
		session.protocolType = ProtocolEthereumStratum

		if len(request.Params) >= 1 {
			userAgent, ok := session.stratumSubscribeRequest.Params[0].(string)
			if ok {
				// Determine if it is a NiceHash client
				if strings.HasPrefix(strings.ToLower(userAgent), niceHashClientTypePrefix) {
					session.isNiceHashClient = true
				}
				// Determine whether it is BTCAgent
				if strings.HasPrefix(strings.ToLower(userAgent), btcAgentClientTypePrefix) {
					session.isBTCAgent = true
					session.protocolType = ProtocolEthereumStratumNiceHash
				}
			}
		}

		if len(request.Params) >= 2 {
			// message example: {"id":1,"method":"mining.subscribe","params":["ethminer 0.15.0rc1","EthereumStratum/1.0.0"]}
			protocol, ok := session.stratumSubscribeRequest.Params[1].(string)

			// "EthereumStratum/xxx"
			if ok && strings.HasPrefix(strings.ToLower(protocol), ethereumStratumNiceHashPrefix) {
				session.protocolType = ProtocolEthereumStratumNiceHash
			}
		}

		result = true
		if session.protocolType == ProtocolEthereumStratumNiceHash {
			extraNonce := session.sessionIDString
			if session.isNiceHashClient {
				// NiceHash Ethereum client currently only supports ExtraNonces up to 2 bytes
				extraNonce = extraNonce[0:4]
			}

			// message example: {"id":1,"jsonrpc":"2.0","result":[["mining.notify","01003f","EthereumStratum/1.0.0"],"01003f"],"error":null}
			result = JSONRPCArray{JSONRPCArray{"mining.notify", session.sessionIDString, ethereumStratumNiceHashVersion}, extraNonce}
		}
		return

	default:
		glog.Fatal("Unknown Chain Type: ", session.manager.chainType)
		err = StratumErrUnknownChainType
		return
	}
}

func (session *StratumSession) makeSubscribeMessageForEthProxy() {
	// Generate a subscription request for the ETHProxy protocol
	// This subscription request is created to send session id, miner IP, etc. to sserver
	session.stratumSubscribeRequest = new(JSONRPCRequest)
	session.stratumSubscribeRequest.Method = "mining.subscribe"
	session.stratumSubscribeRequest.SetParam("ETHProxy", ethproxyVersion)
}

func (session *StratumSession) parseAuthorizeRequest(request *JSONRPCRequest) (result interface{}, err *StratumError) {
	// Save the original request for forwarding to the Stratum server
	session.stratumAuthorizeRequest = request

	// STRATUM / NICEHASH_STRATUM:        {"id":3, "method":"mining.authorize", "params":["test.aaa", "x"]}
	// ETH_PROXY (Claymore):              {"worker": "eth1.0", "jsonrpc": "2.0", "params": ["0x00d8c82Eb65124Ea3452CaC59B64aCC230AA3482.test.aaa", "x"], "id": 2, "method": "eth_submitLogin"}
	// ETH_PROXY (EthMiner, situation 1): {"id":1, "method":"eth_submitLogin", "params":["0x00d8c82Eb65124Ea3452CaC59B64aCC230AA3482"], "worker":"test.aaa"}
	// ETH_PROXY (EthMiner, situation 2): {"id":1, "method":"eth_submitLogin", "params":["test"], "worker":"aaa"}

	if len(request.Params) < 1 {
		err = StratumErrTooFewParams
		return
	}

	fullWorkerName, ok := request.Params[0].(string)

	if !ok {
		err = StratumErrWorkerNameMustBeString
		return
	}

	// miner name
	session.fullWorkerName = FilterWorkerName(fullWorkerName)

	// Ethereum miner names may contain wallet addresses, and the miner name itself may be in an additional worker field
	if session.protocolType != ProtocolBitcoinStratum {
		if request.Worker != "" {
			session.fullWorkerName += "." + FilterWorkerName(request.Worker)
		}
		session.fullWorkerName = StripEthAddrFromFullName(session.fullWorkerName)
	}

	if strings.Contains(session.fullWorkerName, ".") {
		// Intercept before "." as the sub-account name, "." and after as the mining machine name
		pos := strings.Index(session.fullWorkerName, ".")
		session.subaccountName = session.manager.GetRegularSubaccountName(session.fullWorkerName[:pos])
		session.minerNameWithDot = session.fullWorkerName[pos:]
		session.fullWorkerName = session.subaccountName + session.minerNameWithDot
	} else {
		session.subaccountName = session.manager.GetRegularSubaccountName(session.fullWorkerName)
		session.minerNameWithDot = ""
		session.fullWorkerName = session.subaccountName
	}

	if len(session.subaccountName) < 1 {
		err = StratumErrWorkerNameStartWrong
		return
	}

	// Obtaining the name of the miner is successful, but there is no need to return the content to the miner here
	// After connecting to the server, the response sent by the server will be returned to the miner
	result = nil
	err = nil
	return
}

func (session *StratumSession) parseConfigureRequest(request *JSONRPCRequest) (result interface{}, err *StratumError) {
	// request:
	//		{"id":3,"method":"mining.configure","params":[["version-rolling"],{"version-rolling.mask":"1fffe000","version-rolling.min-bit-count":2}]}
	// response:
	//		{"id":3,"result":{"version-rolling":true,"version-rolling.mask":"1fffe000"},"error":null}
	//		{"id":null,"method":"mining.set_version_mask","params":["1fffe000"]}

	if len(request.Params) < 2 {
		err = StratumErrTooFewParams
		return
	}

	if options, ok := request.Params[1].(map[string]interface{}); ok {
		if versionMaskI, ok := options["version-rolling.mask"]; ok {
			if versionMaskStr, ok := versionMaskI.(string); ok {
				versionMask, err := strconv.ParseUint(versionMaskStr, 16, 32)
				if err == nil {
					session.versionMask = uint32(versionMask)
				}
			}
		}
	}

	if session.versionMask != 0 {
		// The response here is a fake version mask. After connecting to the server will pass mining.set_version_mask
		// Update to the real version mask.
		result = JSONRPCObj{
			"version-rolling":      true,
			"version-rolling.mask": session.getVersionMaskStr()}
		return
	}

	// Unknown configuration content, no response
	return
}

func (session *StratumSession) stratumHandleRequest(request *JSONRPCRequest, stat *AuthorizeStat) (result interface{}, err *StratumError) {
	switch request.Method {
	case "mining.subscribe":
		if *stat != StatConnected {
			err = StratumErrDuplicateSubscribed
			return
		}
		result, err = session.parseSubscribeRequest(request)
		if err == nil {
			*stat = StatSubScribed
		}
		return

	case "eth_submitLogin":
		if session.protocolType == ProtocolEthereumProxy {
			session.makeSubscribeMessageForEthProxy()
			*stat = StatSubScribed
			// ETHProxy uses JSON-RPC 2.0
			session.jsonRPCVersion = 2
		}
		fallthrough
	case "mining.authorize":
		if *stat != StatSubScribed {
			err = StratumErrNeedSubscribed
			return
		}
		result, err = session.parseAuthorizeRequest(request)
		if err == nil {
			*stat = StatAuthorized
		}
		return

	case "mining.configure":
		if session.protocolType == ProtocolBitcoinStratum {
			result, err = session.parseConfigureRequest(request)
		}
		return

	default:
		// ignore unimplemented methods
		return
	}
}

func (session *StratumSession) stratumFindWorkerName() error {
	e := make(chan error, 1)

	go func() {
		defer close(e)
		response := new(JSONRPCResponse)

		stat := StatConnected

		// The end of the cycle indicates that the authentication is successful
		for stat != StatAuthorized {
			requestJSON, err := session.clientReader.ReadBytes('\n')

			if err != nil {
				e <- errors.New("read line failed: " + err.Error())
				return
			}

			request, err := NewJSONRPCRequest(requestJSON)

			// ignore the json decode error
			if err != nil {
				if glog.V(3) {
					glog.Info("JSON decode failed: ", err.Error(), string(requestJSON))
				}
				continue
			}

			// stat will be changed in stratumHandleRequest
			result, stratumErr := session.stratumHandleRequest(request, &stat)

			// Both are empty indicating that there is no response you want to return
			if result != nil || stratumErr != nil {
				response.ID = request.ID
				response.Result = result
				response.Error = stratumErr.ToJSONRPCArray(session.manager.serverID)

				_, err = session.writeJSONResponseToClient(response)

				if err != nil {
					e <- errors.New("Write JSON Response Failed: " + err.Error())
					return
				}
			}
		} // for

		// Send an empty error to indicate success
		e <- nil
		return
	}()

	select {
	case err := <-e:
		if err != nil {
			glog.Warning(err)
			return err
		}

		if glog.V(2) {
			glog.Info("FindWorkerName Success: ", session.fullWorkerName)
		}
		return nil

	case <-time.After(findWorkerNameTimeoutSeconds * time.Second):
		glog.Warning("FindWorkerName Timeout")
		return errors.New("FindWorkerName Timeout")
	}
}

func (session *StratumSession) findMiningCoin(autoReg bool) error {
	// Read the currency the user wants to mine from zookeeper
	session.zkWatchPath = session.manager.zookeeperSwitcherWatchDir + session.subaccountName
	data, event, err := session.manager.zookeeperManager.GetW(session.zkWatchPath, session.sessionID)

	if err != nil {
		if autoReg {
			return session.tryAutoReg()
		}

		if glog.V(3) {
			glog.Info("FindMiningCoin Failed: " + session.zkWatchPath + "; " + err.Error())
		}

		var response JSONRPCResponse
		response.Error = NewStratumError(201, "Invalid Sub-account Name").ToJSONRPCArray(session.manager.serverID)
		if session.stratumAuthorizeRequest != nil {
			response.ID = session.stratumAuthorizeRequest.ID
		}

		session.writeJSONResponseToClient(&response)
		return err
	}

	session.miningCoin = string(data)
	session.zkWatchEvent = event

	return nil
}

func (session *StratumSession) tryAutoReg() error {
	glog.Info("Try to auto register sub-account, worker: ", session.fullWorkerName)

	autoRegWatchPath := session.manager.zookeeperAutoRegWatchDir + session.subaccountName
	_, event, err := session.manager.zookeeperManager.GetW(autoRegWatchPath, session.sessionID)
	if err != nil {
		// Check whether the automatic registration wait number exceeds the limit
		if atomic.LoadInt64(&session.manager.autoRegAllowUsers) < 1 {
			glog.Warning("Too much pending auto reg request. worker: ", session.fullWorkerName)
			return ErrTooMuchPendingAutoRegReq
		}
		// There is no lock, and the upper limit is allowed to be exceeded briefly during large concurrency. It is safe to reduce to a negative value
		atomic.AddInt64(&session.manager.autoRegAllowUsers, -1)
		defer atomic.AddInt64(&session.manager.autoRegAllowUsers, 1)

		//--------- Submit a new auto-enrollment request ---------

		type autoRegInfo struct {
			SessionID uint32
			Worker    string
		}

		data := autoRegInfo{session.sessionID, session.fullWorkerName}
		jsonBytes, _ := json.Marshal(data)
		createErr := session.manager.zookeeperManager.Create(autoRegWatchPath, jsonBytes)
		_, event, err = session.manager.zookeeperManager.GetW(autoRegWatchPath, session.sessionID)

		if err != nil {
			if createErr != nil {
				glog.Error("Create auto register key failed, worker: ", session.fullWorkerName, ", errmsg: ", createErr)
			} else {
				glog.Info("Sub-account auto register failed, worker: ", session.fullWorkerName, ", errmsg: ", err)
			}
			return err
		}
	}

	// waiting for register finished for remote process
	<-event

	return session.findMiningCoin(false)
}

func (session *StratumSession) connectStratumServer() error {
	// Get current running status
	runningStat := session.getStatNonLock()
	// Find the server corresponding to the currency
	serverInfo, ok := session.manager.stratumServerInfoMap[session.miningCoin]

	var rpcID interface{}
	if session.stratumAuthorizeRequest != nil {
		rpcID = session.stratumAuthorizeRequest.ID
	}

	// The corresponding server does not exist
	if !ok {
		glog.Error("Stratum Server Not Found: ", session.miningCoin)
		if runningStat != StatReconnecting {
			response := JSONRPCResponse{rpcID, nil, StratumErrStratumServerNotFound.ToJSONRPCArray(session.manager.serverID)}
			session.writeJSONResponseToClient(&response)
		}
		return StratumErrStratumServerNotFound
	}

	// connect to the server
	serverConn, err := net.Dial("tcp", serverInfo.URL)

	if err != nil {
		glog.Error("Connect Stratum Server Failed: ", session.miningCoin, "; ", serverInfo.URL, "; ", err)
		if runningStat != StatReconnecting {
			response := JSONRPCResponse{rpcID, nil, StratumErrConnectStratumServerFailed.ToJSONRPCArray(session.manager.serverID)}
			session.writeJSONResponseToClient(&response)
		}
		return StratumErrConnectStratumServerFailed
	}

	if glog.V(3) {
		glog.Info("Connect Stratum Server Success: ", session.miningCoin, "; ", serverInfo.URL)
	}

	session.serverConn = serverConn
	session.serverReader = bufio.NewReaderSize(serverConn, bufioReaderBufSize)

	return session.serverSubscribeAndAuthorize()
}

// send mining.configure
func (session *StratumSession) sendMiningConfigureToServer() (err error) {
	if session.versionMask == 0 {
		return
	}

	// ask version mask
	request := JSONRPCRequest{
		"configure",
		"mining.configure",
		JSONRPCArray{
			JSONRPCArray{"version-rolling"},
			JSONRPCObj{"version-rolling.mask": session.getVersionMaskStr()}},
		""}
	_, err = session.writeJSONRequestToServer(&request)
	return
}

// send mining.subscribe
func (session *StratumSession) sendMiningSubscribeToServer() (userAgent string, protocol string, err error) {
	userAgent = "stratumSwitcher"
	protocol = "Stratum"

	// copy an object
	request := session.stratumSubscribeRequest
	request.ID = "subscribe"

	// Send subscription message to server
	switch session.protocolType {
	case ProtocolBitcoinStratum:
		// Add sessionID to request
		// API格式：mining.subscribe("user agent/version", "extranonce1")
		// <https://en.bitcoin.it/wiki/Stratum_mining_protocol>

		// get the original parameter 1（user agent）
		if len(session.stratumSubscribeRequest.Params) >= 1 {
			userAgent, _ = session.stratumSubscribeRequest.Params[0].(string)
		}
		if glog.V(3) {
			glog.Info("UserAgent: ", userAgent)
		}

		// In order to ensure the correct display of "Recently Submitted IP" on the web side, pass the IP of the miner as the third parameter to Stratum Server
		clientIP := session.clientIPPort[:strings.LastIndex(session.clientIPPort, ":")]
		clientIPLong := IP2Long(clientIP)
		// Do not use session.sessionIDString directly, because in DCR currency, it has already been padded and reversed.
		sessionIDString := Uint32ToHex(session.sessionID)
		session.stratumSubscribeRequest.SetParam(userAgent, sessionIDString, clientIPLong)

	case ProtocolEthereumStratum:
		fallthrough
	case ProtocolEthereumStratumNiceHash:
		fallthrough
	case ProtocolEthereumProxy:
		// Get the original parameter 1 (user agent) and parameter 2 (protocol, may exist)
		if len(session.stratumSubscribeRequest.Params) >= 1 {
			userAgent, _ = session.stratumSubscribeRequest.Params[0].(string)
		}
		if len(session.stratumSubscribeRequest.Params) >= 2 {
			protocol, _ = session.stratumSubscribeRequest.Params[1].(string)
		}
		if glog.V(3) {
			glog.Info("UserAgent: ", userAgent, "; Protocol: ", protocol)
		}

		clientIP := session.clientIPPort[:strings.LastIndex(session.clientIPPort, ":")]
		clientIPLong := IP2Long(clientIP)

		// Session ID is passed as the third parameter
		// The miner IP is passed as the fourth parameter
		session.stratumSubscribeRequest.SetParam(userAgent, protocol, session.sessionIDString, clientIPLong)

	default:
		glog.Fatal("Unimplemented Stratum Protocol: ", session.protocolType)
		err = ErrParseSubscribeResponseFailed
		return
	}

	// Send a mining.subscribe request to the server
	// The sessionID is already included and sent to the server
	_, err = session.writeJSONRequestToServer(session.stratumSubscribeRequest)
	if err != nil {
		glog.Warning("Write Subscribe Request Failed: ", err)
	}
	return
}

// Sub-account name suffix added when obtaining authentication
func (session *StratumSession) getUserSuffix() string {
	serverInfo, ok := session.manager.stratumServerInfoMap[session.miningCoin]
	if !ok {
		return session.miningCoin
	}

	return serverInfo.UserSuffix
}

// send mining.subscribe
func (session *StratumSession) sendMiningAuthorizeToServer(withSuffix bool) (authWorkerName string, authWorkerPasswd string, err error) {
	if withSuffix {
		// Miner name with coin suffix
		authWorkerName = session.subaccountName + "_" + session.getUserSuffix() + session.minerNameWithDot
	} else {
		// Miner name without currency suffix
		authWorkerName = session.fullWorkerName
	}

	var request JSONRPCRequest
	request.Method = session.stratumAuthorizeRequest.Method
	request.Params = make([]interface{}, len(session.stratumAuthorizeRequest.Params))
	// Deep copy to prevent changes to this parameter from affecting the content of session.stratumAuthorizeRequest
	copy(request.Params, session.stratumAuthorizeRequest.Params)

	// The password sent by the miner, only used for return
	if len(request.Params) >= 2 {
		authWorkerPasswd, _ = request.Params[1].(string)
	}

	//Set to miner name without currency suffix
	request.Params[0] = authWorkerName
	request.ID = "auth"
	// Send a mining.authorize request to the server
	_, err = session.writeJSONRequestToServer(&request)
	return
}

func (session *StratumSession) serverSubscribeAndAuthorize() (err error) {
	// send request
	err = session.sendMiningConfigureToServer()
	if err != nil {
		return
	}
	userAgent, protocol, err := session.sendMiningSubscribeToServer()
	if err != nil {
		return
	}
	authWorkerName, authWorkerPasswd, err := session.sendMiningAuthorizeToServer(false)
	if err != nil {
		return
	}

	// receive response
	e := make(chan error, 1)
	go func() {
		defer close(e)

		var err error
		var allowedVersionMask uint32
		var authResponse JSONRPCResponse
		authMsgCounter := 0
		authSuccess := false

		// The end of the cycle indicates that the authentication is complete
		for authMsgCounter < 2 {
			json, err := session.serverReader.ReadBytes('\n')

			if err != nil {
				e <- errors.New("read line failed: " + err.Error())
				return
			}

			// JSON RPC response returned by the server
			response, err := NewJSONRPCResponse(json)
			// JSON parsing also doesn't fail when the types don't match at all. If the ID is empty, it means notify
			if err == nil && response.ID != nil {
				err = session.stratumHandleServerResponse(response, &authMsgCounter, &authSuccess, &authResponse)
				if err != nil {
					e <- err
					return
				}

				// If the first authentication (without currency suffix) is unsuccessful, send the second authentication request (with currency suffix)
				if !authSuccess && authMsgCounter == 1 {
					authWorkerName, authWorkerPasswd, err = session.sendMiningAuthorizeToServer(true)
					if err != nil {
						e <- err
						return
					}
				}
				continue
			}
			if err != nil && glog.V(3) {
				glog.Info("JSON RPC Response decode failed: ", err.Error(), string(json))
			}

			// Server Pushed JSON RPC Notifications
			notify, err := NewJSONRPCRequest(json)
			if err == nil {
				err = session.stratumHandleServerNotify(notify, &allowedVersionMask)
				if err != nil {
					e <- err
					return
				}
				continue
			}
			if err != nil && glog.V(3) {
				glog.Info("JSON RPC Request decode failed: ", err.Error(), string(json))
			}
		} // for

		// Send authentication response to miner
		authResponse.ID = session.stratumAuthorizeRequest.ID
		_, err = session.writeJSONResponseToClient(&authResponse)
		if err != nil {
			e <- err
			return
		}

		// Send version mask updates
		if authSuccess && session.versionMask != 0 {
			allowedVersionMask &= session.versionMask
			notify := JSONRPCRequest{
				nil,
				"mining.set_version_mask",
				JSONRPCArray{fmt.Sprintf("%08x", allowedVersionMask)},
				""}
			_, err = session.writeJSONNotifyToClient(&notify)
			if err != nil {
				e <- err
				return
			}
		}

		if !authSuccess {
			err = errors.New("Authorize Failed for Server")
		}
		// Send the authentication result, nil means success
		e <- err
		return
	}()

	select {
	case err = <-e:
		if err != nil {
			if glog.V(2) {
				glog.Warning("Authorize Failed: ", session.clientIPPort, "; ", session.miningCoin, "; ",
					authWorkerName, "; ", authWorkerPasswd, "; ", userAgent, ";",
					session.getVersionMaskStr(), "; ", protocol, "; ", err)
			}
		} else {
			if glog.V(2) {
				glog.Info("Authorize Success: ", session.clientIPPort, "; ", session.miningCoin, "; ",
					authWorkerName, "; ", authWorkerPasswd, "; ", userAgent, "; ",
					session.getVersionMaskStr(), "; ", protocol)
			}
		}

	case <-time.After(readServerResponseTimeoutSeconds * time.Second):
		err = errors.New("Authorize Timeout")
		glog.Warning(err)
	}

	return
}

// Handling server notifications
func (session *StratumSession) stratumHandleServerNotify(notify *JSONRPCRequest, allowedVersionMask *uint32) (err error) {
	switch notify.Method {
	case "mining.set_version_mask":
		if len(notify.Params) >= 1 {
			if versionMaskStr, ok := notify.Params[0].(string); ok {
				mask, err := strconv.ParseUint(versionMaskStr, 16, 32)
				if err == nil {
					*allowedVersionMask = uint32(mask)
				}
			}
		}
	}
	return
}

// Handling server responses
func (session *StratumSession) stratumHandleServerResponse(response *JSONRPCResponse, authMsgCounter *int, authSuccess *bool, authResponse *JSONRPCResponse) (err error) {
	id, ok := response.ID.(string)
	if !ok {
		glog.Warning("Server Response ID is Not a String: ", response)
		return
	}

	switch id {
	case "configure":
		// ignore

	case "subscribe":
		err = session.stratumHandleServerSubscribeResponse(response)

	case "auth":
		*authMsgCounter++
		success := session.stratumHandleServerAuthorizeResponse(response)
		if success || !(*authSuccess) {
			*authResponse = *response
		}
		if success {
			*authSuccess = true
			*authMsgCounter = 2 // Authentication has succeeded, no further authentication requests need to be sent
		}
	}
	return
}

// Handling server authentication responses
func (session *StratumSession) stratumHandleServerAuthorizeResponse(response *JSONRPCResponse) bool {
	success, ok := response.Result.(bool)
	return ok && success
}

// Handling server subscription responses
func (session *StratumSession) stratumHandleServerSubscribeResponse(response *JSONRPCResponse) error {
	// Check the subscription result returned by the server
	switch session.protocolType {
	case ProtocolBitcoinStratum:
		result, ok := response.Result.([]interface{})
		if !ok {
			glog.Warning("Parse Subscribe Response Failed: result is not an array")
			return ErrParseSubscribeResponseFailed
		}
		if len(result) < 2 {
			glog.Warning("Field too Few of Subscribe Response Result: ", result)
			return ErrParseSubscribeResponseFailed
		}

		sessionID, ok := result[1].(string)
		if !ok {
			glog.Warning("Parse Subscribe Response Failed: result[1] is not a string")
			return ErrParseSubscribeResponseFailed
		}

		// returned by the server The sessionID is inconsistent with the currently saved, all the shares dug up at this time will be invalid, and the connection will be disconnected
		if sessionID != session.sessionIDString {
			glog.Warning("Session ID Mismatched:  ", sessionID, " != ", session.sessionIDString)
			return ErrSessionIDInconformity
		}

	case ProtocolEthereumStratumNiceHash:
		result, ok := response.Result.([]interface{})
		if !ok {
			glog.Warning("Parse Subscribe Response Failed: result is not an array")
			return ErrParseSubscribeResponseFailed
		}
		if len(result) < 2 {
			glog.Warning("Field too Few of Subscribe Response Result: ", result)
			return ErrParseSubscribeResponseFailed
		}

		notify, ok := result[0].([]interface{})
		if !ok {
			glog.Warning("Parse Subscribe Response Failed: result[0] is not a array")
			return ErrParseSubscribeResponseFailed
		}

		sessionID, ok := notify[1].(string)
		if !ok {
			glog.Warning("Parse Subscribe Response Failed: result[0][1] is not a string")
			return ErrParseSubscribeResponseFailed
		}

		sessionExtraNonce := session.sessionIDString
		if session.isNiceHashClient {
			sessionExtraNonce = sessionExtraNonce[0:4]
		}
		extraNonce, ok := result[1].(string)
		if !ok {
			glog.Warning("Parse Subscribe Response Failed: result[1] is not a string")
			return ErrParseSubscribeResponseFailed
		}

		// The sessionID returned by the server is inconsistent with the currently saved session ID. All shares mined at this time will be invalid and the connection will be disconnected.
		if sessionID != session.sessionIDString {
			glog.Warning("Session ID Mismatched:  ", sessionID, " != ", session.sessionIDString)
			return ErrSessionIDInconformity
		}
		if extraNonce != sessionExtraNonce {
			glog.Warning("ExtraNonce Mismatched:  ", extraNonce, " != ", sessionExtraNonce)
			return ErrSessionIDInconformity
		}

	case ProtocolEthereumStratum:
		fallthrough
	case ProtocolEthereumProxy:
		result, ok := response.Result.(bool)
		if !ok || !result {
			glog.Warning("Parse Subscribe Response Failed: response is ", response)
			return ErrParseSubscribeResponseFailed
		}

	default:
		glog.Fatal("Unimplemented Stratum Protocol: ", session.protocolType)
		return ErrParseSubscribeResponseFailed
	}

	if glog.V(3) {
		glog.Info("Subscribe Success: ", response)
	}
	return nil
}

func (session *StratumSession) proxyStratum() {
	if session.getStat() != StatRunning {
		glog.Info("proxyStratum: session stopped by another goroutine")
		return
	}

	// Register for a session
	session.manager.RegisterStratumSession(session)

	// From server to client
	go func() {
		// Record the current currency switch count
		currentReconnectCounter := session.getReconnectCounter()

		if session.serverReader != nil {
			bufLen := session.serverReader.Buffered()
			// Write the remaining content in bufio to the peer
			if bufLen > 0 {
				buf := make([]byte, bufLen)
				session.serverReader.Read(buf)
				session.clientConn.Write(buf)
			}
			// release bufio
			session.serverReader = nil
		}
		// simple streaming replication
		buffer := make([]byte, bufioReaderBufSize)
		_, err := IOCopyBuffer(session.clientConn, session.serverConn, buffer)
		// Streaming replication ends, indicating that one of the parties has closed the connection
		// Do not reconnect to the BTCAgent application
		if err == ErrReadFailed && !session.isBTCAgent {
			// 服务器关闭了连接，尝试重连
			session.tryReconnect(currentReconnectCounter)
		} else {
			// The client closed the connection, ending the session
			session.tryStop(currentReconnectCounter)
		}
		if glog.V(3) {
			glog.Info("DownStream: exited; ", session.clientIPPort, "; ", session.fullWorkerName, "; ", session.miningCoin)
		}
	}()

	// From client to server
	go func() {
		// Record the current currency switch count
		currentReconnectCounter := session.getReconnectCounter()

		if session.clientReader != nil {
			bufLen := session.clientReader.Buffered()
			// Write the remaining content in bufio to the peer
			if bufLen > 0 {
				buf := make([]byte, bufLen)
				session.clientReader.Read(buf)
				session.serverConn.Write(buf)
			}
			// release bufio
			session.clientReader = nil
		}
		// simple streaming replication
		buffer := make([]byte, bufioReaderBufSize)
		bufferLen, err := IOCopyBuffer(session.serverConn, session.clientConn, buffer)
		// Streaming replication ends, indicating that one of the parties has closed the connection
		// Do not reconnect to the BTCAgent application
		if err == ErrWriteFailed && !session.isBTCAgent {
			// 服务器关闭了连接，尝试重连
			session.tryReconnect(currentReconnectCounter)
			// getStat() will lock until the reconnection succeeds or the reconnection is abandoned
			// If the reconnection is successful, try to forward the content in the cache to the new server
			if bufferLen > 0 && session.getStat() == StatRunning {
				session.serverConn.Write(buffer[0:bufferLen])
			}
		} else {
			// The client closed the connection, ending the session
			session.tryStop(currentReconnectCounter)
		}
		if glog.V(3) {
			glog.Info("UpStream: exited; ", session.clientIPPort, "; ", session.fullWorkerName, "; ", session.miningCoin)
		}
	}()

	// Monitor switching instructions from zookeeper and do Stratum switching
	go func() {
		// Record the current currency switch count
		currentReconnectCounter := session.getReconnectCounter()

		for {
			<-session.zkWatchEvent

			if !session.IsRunning() {
				break
			}

			if currentReconnectCounter != session.getReconnectCounter() {
				break
			}

			data, event, err := session.manager.zookeeperManager.GetW(session.zkWatchPath, session.sessionID)

			if err != nil {
				glog.Error("Read From Zookeeper Failed, sleep ", zookeeperConnAliveTimeout, "s: ", session.zkWatchPath, "; ", err)
				time.Sleep(zookeeperConnAliveTimeout * time.Second)
				continue
			}

			session.zkWatchEvent = event
			newMiningCoin := string(data)

			// If the currency has not changed, continue monitoring
			if newMiningCoin == session.miningCoin {
				if glog.V(3) {
					glog.Info("Mining Coin Not Changed: ", session.fullWorkerName, ": ", session.miningCoin, " -> ", newMiningCoin)
				}
				continue
			}

			// If the Stratum server corresponding to the currency does not exist, ignore the event and continue monitoring
			_, exists := session.manager.stratumServerInfoMap[newMiningCoin]
			if !exists {
				glog.Error("Stratum Server Not Found for New Mining Coin: ", newMiningCoin)
				continue
			}

			// Currency changed
			if glog.V(2) {
				glog.Info("Mining Coin Changed: ", session.fullWorkerName, "; ", session.miningCoin, " -> ", newMiningCoin, "; ", currentReconnectCounter)
			}

			// perform currency switch
			if session.isBTCAgent {
				// Because BTCAgent sessions are stateful (a connection contains multiple AgentSessions,
				// Corresponding to multiple miners), so there is no way to safely switch BTCAgent sessions seamlessly,
				// Only the disconnect method can be used.
				session.tryStop(currentReconnectCounter)
			} else {
				// Common connection, direct currency switch
				session.switchCoinType(newMiningCoin, currentReconnectCounter)
			}
			break
		}

		if glog.V(3) {
			glog.Info("CoinWatcher: exited; ", session.clientIPPort, "; ", session.fullWorkerName, "; ", session.miningCoin)
		}
	}()
}

// Check if a reconnection has occurred, if not, stop the session
func (session *StratumSession) tryStop(currentReconnectCounter uint32) bool {
	session.lock.Lock()
	defer session.lock.Unlock()

	// Session not running, not stopped
	if session.runningStat != StatRunning {
		return false
	}

	// Determine if it has been reconnected
	if currentReconnectCounter == session.reconnectCounter {
		//未发生重连，尝试停止
		go session.Stop()
		return true
	}

	// Reconnected, do nothing
	return false
}

// Check if a reconnection has occurred, if not, try to reconnect
func (session *StratumSession) tryReconnect(currentReconnectCounter uint32) bool {
	session.lock.Lock()
	defer session.lock.Unlock()

	// Session not running, do not reconnect
	if session.runningStat != StatRunning {
		return false
	}

	// Determine if it has been reconnected
	if currentReconnectCounter == session.reconnectCounter {
		//Reconnection did not occur, try reconnection
		// The status is set to "reconnecting to the server", and the reconnection counter is incremented by one
		session.setStatNonLock(StatReconnecting)
		session.reconnectCounter++

		if glog.V(3) {
			glog.Info("Reconnect Server: ", session.clientIPPort, "; ", session.fullWorkerName, "; ", session.miningCoin)
		}

		session.reconnectStratumServer(retryTimeWhenServerDown)
		return true
	}

	// Reconnected, do nothing
	return false
}

func (session *StratumSession) switchCoinType(newMiningCoin string, currentReconnectCounter uint32) {
	// Set new currency
	session.miningCoin = newMiningCoin

	// Lock the session to prevent it from being stopped by other threads
	session.lock.Lock()
	defer session.lock.Unlock()

	// Session not running, abandon operation
	if session.runningStat != StatRunning {
		glog.Warning("SwitchCoinType: session not running")
		return
	}
	// The session has been reconnected by another thread, giving up the operation
	if currentReconnectCounter != session.reconnectCounter {
		glog.Warning("SwitchCoinType: session reconnected by other goroutine")
		return
	}
	// Session not reconnected, operational
	// The status is set to "reconnecting to the server", and the reconnection counter is incremented by one
	session.setStatNonLock(StatReconnecting)
	session.reconnectCounter++

	// reconnect server
	session.reconnectStratumServer(retryTimeWhenServerDown)
}

// reconnectStratumServer reconnect server
func (session *StratumSession) reconnectStratumServer(retryTime int) {
	// remove session registration
	session.manager.UnRegisterStratumSession(session)

	// destroy serverReader
	if session.serverReader != nil {
		bufLen := session.serverReader.Buffered()
		// Write the remaining content in bufio to the peer
		if bufLen > 0 {
			buf := make([]byte, bufLen)
			session.serverReader.Read(buf)
			session.clientConn.Write(buf)
		}
		session.serverReader = nil
	}

	// Disconnect the original server
	session.serverConn.Close()
	session.serverConn = nil

	// recreate clientReader
	if session.clientReader == nil {
		session.clientReader = bufio.NewReaderSize(session.clientConn, bufioReaderBufSize)
	}

	// connect to the server
	var err error
	// At least try it once, so start with -1
	for i := -1; i < retryTime; i++ {
		err = session.connectStratumServer()
		if err == nil {
			break
		} else {
			time.Sleep(1 * time.Second)
		}
	}
	if err != nil {
		if glog.V(2) {
			glog.Info("Reconnect Server Failed: ", session.clientIPPort, "; ", session.fullWorkerName, "; ", session.miningCoin, "; ", err)
		}
		go session.Stop()
		return
	}

	// back to running
	session.setStatNonLock(StatRunning)

	// Switch to pure proxy mode
	go session.proxyStratum()

	if glog.V(2) {
		glog.Info("Reconnect Server Success: ", session.clientIPPort, "; ", session.fullWorkerName, "; ", session.miningCoin)
	}
}

func peekWithTimeout(reader *bufio.Reader, len int, timeout time.Duration) ([]byte, error) {
	e := make(chan error, 1)
	var buffer []byte

	go func() {
		data, err := reader.Peek(len)
		buffer = data
		e <- err
		close(e)
	}()

	select {
	case err := <-e:
		return buffer, err
	case <-time.After(timeout):
		return nil, ErrBufIOReadTimeout
	}
}

func (session *StratumSession) peekFromClientWithTimeout(len int, timeout time.Duration) ([]byte, error) {
	return peekWithTimeout(session.clientReader, len, timeout)
}

func (session *StratumSession) peekFromServerWithTimeout(len int, timeout time.Duration) ([]byte, error) {
	return peekWithTimeout(session.serverReader, len, timeout)
}

func readByteWithTimeout(reader *bufio.Reader, buffer []byte, timeout time.Duration) (int, error) {
	e := make(chan error, 1)
	var length int

	go func() {
		len, err := reader.Read(buffer)
		length = len
		e <- err
		close(e)
	}()

	select {
	case err := <-e:
		return length, err
	case <-time.After(timeout):
		return 0, ErrBufIOReadTimeout
	}
}

func readLineWithTimeout(reader *bufio.Reader, timeout time.Duration) ([]byte, error) {
	e := make(chan error, 1)
	var buffer []byte

	go func() {
		data, err := reader.ReadBytes('\n')
		buffer = data
		e <- err
		close(e)
	}()

	select {
	case err := <-e:
		return buffer, err
	case <-time.After(timeout):
		return nil, ErrBufIOReadTimeout
	}
}

func (session *StratumSession) readByteFromClientWithTimeout(buffer []byte, timeout time.Duration) (int, error) {
	return readByteWithTimeout(session.clientReader, buffer, timeout)
}

func (session *StratumSession) readByteFromServerWithTimeout(buffer []byte, timeout time.Duration) (int, error) {
	return readByteWithTimeout(session.serverReader, buffer, timeout)
}

func (session *StratumSession) readLineFromClientWithTimeout(timeout time.Duration) ([]byte, error) {
	return readLineWithTimeout(session.clientReader, timeout)
}

func (session *StratumSession) readLineFromServerWithTimeout(timeout time.Duration) ([]byte, error) {
	return readLineWithTimeout(session.serverReader, timeout)
}

func (session *StratumSession) writeJSONNotifyToClient(jsonData *JSONRPCRequest) (int, error) {
	bytes, err := jsonData.ToJSONBytes()

	if err != nil {
		return 0, err
	}

	defer session.clientConn.Write([]byte{'\n'})
	return session.clientConn.Write(bytes)
}

func (session *StratumSession) writeJSONResponseToClient(jsonData *JSONRPCResponse) (int, error) {
	bytes, err := jsonData.ToJSONBytes(session.jsonRPCVersion)

	if err != nil {
		return 0, err
	}

	defer session.clientConn.Write([]byte{'\n'})
	return session.clientConn.Write(bytes)
}

func (session *StratumSession) writeJSONRequestToServer(jsonData *JSONRPCRequest) (int, error) {
	bytes, err := jsonData.ToJSONBytes()

	if err != nil {
		return 0, err
	}

	defer session.serverConn.Write([]byte{'\n'})
	return session.serverConn.Write(bytes)
}

func (session *StratumSession) getVersionMaskStr() string {
	return fmt.Sprintf("%08x", session.versionMask)
}
