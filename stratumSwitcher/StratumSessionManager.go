package main

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/samuel/go-zookeeper/zk"
	"github.com/willf/bitset"
)

// StratumServerInfo Information on Stratum Servers
type StratumServerInfo struct {
	URL        string
	UserSuffix string
}

// StratumServerInfoMap Hash table of information for Stratum servers
type StratumServerInfoMap map[string]StratumServerInfo

// StratumSessionMap Stratum session hash table
type StratumSessionMap map[uint32]*StratumSession

// StratumSessionManager Stratum Session Manager
type StratumSessionManager struct {
	// The lock added when modifying StratumSessionMap
	lock sync.Mutex
	// All sessions in normal proxy state
	sessions StratumSessionMap
	// Session ID Manager
	sessionIDManager *SessionIDManager
	// Stratum Server List
	stratumServerInfoMap StratumServerInfoMap
	// Zookeeper Manager
	zookeeperManager *ZookeeperManager
	// zookeeperSwitcherWatchDir The zookeeper directory path monitored by the switch service
	// The specific monitoring path is zookeeperSwitcherWatchDir/sub account name
	zookeeperSwitcherWatchDir string
	// enableUserAutoReg Whether to open the sub-account automatic registration function
	enableUserAutoReg bool
	// zookeeperAutoRegWatchDir Zookeeper directory path for automatic registration service monitoring
	// The specific monitoring path is zookeeperAutoRegWatchDir/sub account name
	zookeeperAutoRegWatchDir string
	// The number of auto-registered users currently allowed (1 minus 1 for registration, add back after completion, and 0 to reject auto-registration to prevent DDoS)
	autoRegAllowUsers int64
	// stratum The server is not case sensitive to the sub-account name
	stratumServerCaseInsensitive bool
	// Case-insensitive username index (nullable, only used when stratumServerCaseInsensitive == false)
	zkUserCaseInsensitiveIndex string
	// Listening IP and TCP port
	tcpListenAddr string
	// TCP listener object
	tcpListener net.Listener
	// Upgrading objects without downtime
	upgradable *Upgradable
	// blockchain type
	chainType ChainType
	// serverID to display in error messages
	serverID uint8
}

// NewStratumSessionManager Create Stratum Session Manager
func NewStratumSessionManager(conf ConfigData, runtimeData RuntimeData) (manager *StratumSessionManager, err error) {
	var chainType ChainType
	var indexBits uint8

	switch strings.ToLower(conf.ChainType) {
	case "bitcoin":
		chainType = ChainTypeBitcoin
		indexBits = 24
	case "decred-normal":
		chainType = ChainTypeDecredNormal
		indexBits = 24
	case "decred-gominer":
		chainType = ChainTypeDecredGoMiner
		indexBits = 24
	case "ethereum":
		chainType = ChainTypeEthereum
		indexBits = 16
	default:
		err = errors.New("Unknown ChainType: " + conf.ChainType)
		return
	}

	manager = new(StratumSessionManager)

	manager.serverID = conf.ServerID
	manager.sessions = make(StratumSessionMap)
	manager.stratumServerInfoMap = conf.StratumServerMap
	manager.zookeeperSwitcherWatchDir = conf.ZKSwitcherWatchDir
	manager.enableUserAutoReg = conf.EnableUserAutoReg
	manager.zookeeperAutoRegWatchDir = conf.ZKAutoRegWatchDir
	manager.autoRegAllowUsers = conf.AutoRegMaxWaitUsers
	manager.stratumServerCaseInsensitive = conf.StratumServerCaseInsensitive
	manager.zkUserCaseInsensitiveIndex = conf.ZKUserCaseInsensitiveIndex
	manager.tcpListenAddr = conf.ListenAddr
	manager.chainType = chainType

	manager.zookeeperManager, err = NewZookeeperManager(conf.ZKBroker)
	if err != nil {
		return
	}

	if manager.serverID == 0 {
		// try to assign id from zookeeper
		manager.serverID, err = manager.AssignServerIDFromZK(conf.ZKServerIDAssignDir, runtimeData.ServerID)
		if err != nil {
			err = errors.New("Cannot assign server id from zk: " + err.Error())
			return
		}
	}

	manager.sessionIDManager, err = NewSessionIDManager(manager.serverID, indexBits)
	if err != nil {
		return
	}

	if manager.chainType == ChainTypeEthereum {
		// By default, a larger ID allocation interval is adopted to reduce the impact of overlapping mining space.
		//Since the SessionID is pre-allocated, for compatibility with the NiceHash Ethereum client that requires an extraNonce of no more than 2 bytes,
		manager.sessionIDManager.setAllocInterval(256)
	}

	return
}

// AssignServerIDFromZK Assign server ID from Zookeeper
func (manager *StratumSessionManager) AssignServerIDFromZK(assignDir string, oldServerID uint8) (serverID uint8, err error) {
	manager.zookeeperManager.createZookeeperPath(assignDir)

	parent := assignDir[:len(assignDir)-1]
	var children []string
	children, _, err = manager.zookeeperManager.zookeeperConn.Children(parent)
	if err != nil {
		return
	}

	childrenSet := bitset.New(256)
	childrenSet.Set(0) // id 0 not assignable
	// Record the assigned id into the bitset
	for _, idStr := range children {
		idInt, convErr := strconv.Atoi(idStr)
		if convErr != nil {
			glog.Warning("AssignServerIDFromZK: strconv.Atoi(", idStr, ") failed. errmsg: ", convErr)
			continue
		}
		if idInt < 1 || idInt > 255 {
			glog.Warning("AssignServerIDFromZK: found out of range id in zk: ", idStr)
			continue
		}
		childrenSet.Set(uint(idInt))
	}

	// Construct the meta information written to the allocation node
	type SwitcherMetaData struct {
		ChainType  string
		Coins      []string
		IPs        []string
		HostName   string
		ListenAddr string
	}
	var data SwitcherMetaData
	data.ChainType = manager.chainType.ToString()
	data.HostName, _ = os.Hostname()
	data.ListenAddr = manager.tcpListenAddr
	for coin := range manager.stratumServerInfoMap {
		data.Coins = append(data.Coins, coin)
	}
	if ips, err := net.InterfaceAddrs(); err == nil {
		for _, ip := range ips {
			data.IPs = append(data.IPs, ip.String())
		}
	}

	dataJSON, _ := json.Marshal(data)

	// Find and try assignable id
	idIndex := uint(oldServerID)
	for {
		newID, success := childrenSet.NextClear(idIndex)
		if !success {
			err = errors.New("server id is full")
			return
		}

		nodePath := assignDir + strconv.Itoa(int(newID))
		_, err = manager.zookeeperManager.zookeeperConn.Create(nodePath, dataJSON, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
		if err != nil {
			glog.Warning("AssignServerIDFromZK: create ", nodePath, " failed. errmsg: ", err)
			continue
		}

		glog.Info("AssignServerIDFromZK: got server id ", newID, " (", nodePath, ")")
		serverID = uint8(newID)
		return
	}
}

// RunStratumSession Run a Stratum session
func (manager *StratumSessionManager) RunStratumSession(conn net.Conn) {
	// 产生 sessionID （Extranonce1）
	sessionID, err := manager.sessionIDManager.AllocSessionID()

	if err != nil {
		conn.Close()
		glog.Error("NewStratumSession failed: ", err)
		return
	}

	session := NewStratumSession(manager, conn, sessionID)
	session.Run()
}

// ResumeStratumSession Resume a Stratum session
func (manager *StratumSessionManager) ResumeStratumSession(sessionData StratumSessionData) {
	clientConn, clientErr := newConnFromFd(sessionData.ClientConnFD)
	serverConn, serverErr := newConnFromFd(sessionData.ServerConnFD)

	if clientErr != nil {
		glog.Error("Resume client conn failed: ", clientErr)
		return
	}

	if serverErr != nil {
		glog.Error("Resume server conn failed: ", clientErr)
		return
	}

	if clientConn.RemoteAddr() == nil {
		glog.Error("Resume client conn failed: downstream exited.")
		return
	}

	if serverConn.RemoteAddr() == nil {
		glog.Error("Resume client conn failed: upstream exited.")
		return
	}

	//restore sessionID
	err := manager.sessionIDManager.ResumeSessionID(sessionData.SessionID)
	if err != nil {
		glog.Error("Resume server conn failed: ", err)
	}

	session := NewStratumSession(manager, clientConn, sessionData.SessionID)
	session.Resume(sessionData, serverConn)
}

// RegisterStratumSession Register Stratum session (called after Stratum session starts normal proxy)
func (manager *StratumSessionManager) RegisterStratumSession(session *StratumSession) {
	manager.lock.Lock()
	manager.sessions[session.sessionID] = session
	manager.lock.Unlock()
}

// UnRegisterStratumSession Unregister Stratum session (called when Stratum session is reconnected)
func (manager *StratumSessionManager) UnRegisterStratumSession(session *StratumSession) {
	manager.lock.Lock()
	// delete a registered session
	delete(manager.sessions, session.sessionID)
	manager.lock.Unlock()

	// Remove currency monitoring from Zookeeper manager
	manager.zookeeperManager.ReleaseW(session.zkWatchPath, session.sessionID)
}

// ReleaseStratumSession Release Stratum session (called when Stratum session is stopped)
func (manager *StratumSessionManager) ReleaseStratumSession(session *StratumSession) {
	manager.lock.Lock()
	// delete a registered session
	delete(manager.sessions, session.sessionID)
	manager.lock.Unlock()

	// release session id
	manager.sessionIDManager.FreeSessionID(session.sessionID)
	// Remove currency monitoring from Zookeeper manager
	manager.zookeeperManager.ReleaseW(session.zkWatchPath, session.sessionID)
}

// Run Start running the StratumSwitcher service
func (manager *StratumSessionManager) Run(runtimeData RuntimeData) {
	var err error

	if runtimeData.Action == "upgrade" {
		// Resume TCP session
		for _, sessionData := range runtimeData.SessionDatas {
			manager.ResumeStratumSession(sessionData)
		}
	}

	// TCP listening
	glog.Info("Listen TCP ", manager.tcpListenAddr)
	manager.tcpListener, err = net.Listen("tcp", manager.tcpListenAddr)

	if err != nil {
		glog.Fatal("listen failed: ", err)
		return
	}

	manager.Upgradable()

	for {
		conn, err := manager.tcpListener.Accept()

		if err != nil {
			continue
		}

		go manager.RunStratumSession(conn)
	}
}

// Upgradable Enables StratumSwitcher upgrades without downtime
func (manager *StratumSessionManager) Upgradable() {
	manager.upgradable = NewUpgradable(manager)

	go signalUSR2Listener(func() {
		err := manager.upgradable.upgradeStratumSwitcher()
		if err != nil {
			glog.Error("Upgrade Failed: ", err)
		}
	})

	glog.Info("Stratum Switcher is Now Upgradable.")
}

// GetRegularSubaccountName get normalized(大小写敏感的)子账户名
func (manager *StratumSessionManager) GetRegularSubaccountName(subAccountName string) string {
	if manager.stratumServerCaseInsensitive {
		// The server is insensitive to the case of the sub-account name, and directly returns the lower-case sub-account name
		return strings.ToLower(subAccountName)
	}

	if len(manager.zkUserCaseInsensitiveIndex) <= 0 {
		// zkUserCaseInsensitiveIndex Disabled (empty), the sub-account name itself is returned directly
		return subAccountName
	}

	path := manager.zkUserCaseInsensitiveIndex + strings.ToLower(subAccountName)
	regularNameBytes, _, err := manager.zookeeperManager.zookeeperConn.Get(path)
	if err != nil {
		if glog.V(3) {
			glog.Info("GetRegularSubaccountName failed. user: ", subAccountName, ", errmsg: ", err)
		}
		return subAccountName
	}
	regularName := string(regularNameBytes)
	if glog.V(3) {
		glog.Info("GetRegularSubaccountName: ", subAccountName, " -> ", regularName)
	}
	return regularName
}
