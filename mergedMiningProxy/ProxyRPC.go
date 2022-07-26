package main

import (
	"crypto/subtle"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"merkle-tree-and-bitcoin/hash"

	"github.com/golang/glog"
)

// RPCResultCreateAuxBlock The return result of the RPC method createauxblock
type RPCResultCreateAuxBlock struct {
	Hash          string `json:"hash"`
	ChainID       uint32 `json:"chainid"`
	PrevBlockHash string `json:"previousblockhash"`
	CoinbaseValue uint64 `json:"coinbasevalue"`
	Bits          string `json:"bits"`
	Height        uint32 `json:"height"`
	Target        string `json:"_target"`
	MerkleSize    uint32 `json:"merkle_size"`
	MerkleNonce   uint32 `json:"merkle_nonce"`
}

// write Output information in JSON-RPC format
func write(w http.ResponseWriter, response interface{}) {
	responseJSON, _ := json.Marshal(response)
	w.Write(responseJSON)
}

// writeError Output error messages in JSON-RPC format
func writeError(w http.ResponseWriter, id interface{}, errNo int, errMsg string) {
	err := RPCError{errNo, errMsg}
	response := RPCResponse{id, nil, err}
	write(w, response)
}

// ProxyRPCHandle Proxy RPC handler
type ProxyRPCHandle struct {
	config      ProxyRPCServer
	auxJobMaker *AuxJobMaker
	dbhandle    DBConnection
}

// NewProxyRPCHandle Create a proxy RPC handler
func NewProxyRPCHandle(config ProxyRPCServer, auxJobMaker *AuxJobMaker) (handle *ProxyRPCHandle) {
	handle = new(ProxyRPCHandle)
	handle.config = config
	handle.auxJobMaker = auxJobMaker
	handle.dbhandle.InitDB(config.PoolDb)
	return
}

// basicAuth Perform Basic authentication
func (handle *ProxyRPCHandle) basicAuth(r *http.Request) bool {
	apiUser := []byte(handle.config.User)
	apiPasswd := []byte(handle.config.Passwd)

	user, passwd, ok := r.BasicAuth()

	// Check if the username and password are correct
	if ok && subtle.ConstantTimeCompare(apiUser, []byte(user)) == 1 && subtle.ConstantTimeCompare(apiPasswd, []byte(passwd)) == 1 {
		return true
	}

	return false
}

func (handle *ProxyRPCHandle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !handle.basicAuth(r) {
		// Authentication failed with 401 Unauthorized
		// Restricted can be changed to other values
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		// 401 status code
		w.WriteHeader(http.StatusUnauthorized)
		// 401 page
		w.Write([]byte(`<h1>401 - Unauthorized</h1>`))
		return
	}

	if r.Method != "POST" {
		w.Write([]byte("JSONRPC server handles only POST requests"))
		return
	}

	requestJSON, err := ioutil.ReadAll(r.Body)
	if err != nil {
		writeError(w, nil, 400, err.Error())
		return
	}

	var request RPCRequest
	err = json.Unmarshal(requestJSON, &request)
	if err != nil {
		writeError(w, nil, 400, err.Error())
		return
	}

	response := RPCResponse{request.ID, nil, nil}

	switch request.Method {
	case "createauxblock":
		handle.createAuxBlock(&response)
	case "submitauxblock":
		handle.submitAuxBlock(request.Params, &response)
	case "getauxblock":
		if len(request.Params) > 0 {
			handle.submitAuxBlock(request.Params, &response)
		} else {
			handle.createAuxBlock(&response)
		}
	default:
		// Forward the unknown method to the server of the first chain
		responseJSON, err := RPCCall(handle.auxJobMaker.chains[0].RPCServer, request.Method, request.Params)
		if err != nil {
			writeError(w, nil, 400, err.Error())
			return
		}
		response, err = ParseRPCResponse(responseJSON)
		if err != nil {
			writeError(w, nil, 400, err.Error())
			return
		}
		// If the help method is called, the description of createauxblock and submitauxblock is appended to the result
		if request.Method == "help" && len(request.Params) == 0 {
			helpStr, ok := response.Result.(string)
			if ok {
				helpStr += "\n\n== Merged Mining Proxy ==\n" +
					"createauxblock <address>\n" +
					"submitauxblock <hash> <auxpow>\n" +
					"getauxblock (hash auxpow)"
				response.Result = helpStr
			}
		}
	}

	write(w, response)
}

func (handle *ProxyRPCHandle) createAuxBlock(response *RPCResponse) {
	job, err := handle.auxJobMaker.GetAuxJob()
	if err != nil {
		response.Error = RPCError{500, err.Error()}
		return
	}

	var result RPCResultCreateAuxBlock
	result.Bits = job.MinBits
	result.ChainID = 1
	result.CoinbaseValue = job.CoinbaseValue
	result.Hash = job.MerkleRoot.HexReverse()
	result.Height = job.Height
	result.PrevBlockHash = job.PrevBlockHash.Hex()
	result.Target = job.MaxTarget.HexReverse()
	result.MerkleSize = job.MerkleSize
	result.MerkleNonce = job.MerkleNonce

	glog.Info("[CreateAuxBlock] height:", result.Height,
		", bits:", result.Bits,
		", target:", job.MaxTarget.Hex(),
		", coinbaseValue:", result.CoinbaseValue,
		", hash:", job.MerkleRoot.Hex(),
		", prevHash:", result.PrevBlockHash,
		", merkleSize: ", job.MerkleSize,
		", merkleNonce: ", job.MerkleNonce)

	response.Result = result
	return
}

func (handle *ProxyRPCHandle) submitAuxBlock(params []interface{}, response *RPCResponse) {
	if len(params) < 2 {
		response.Error = RPCError{400, "The number of params should be 2"}
		return
	}

	hashHex, ok := params[0].(string)
	if !ok {
		response.Error = RPCError{400, "The param 1 should be a string"}
		return
	}

	auxPowHex, ok := params[1].(string)
	if !ok {
		response.Error = RPCError{400, "The param 2 should be a string"}
		return
	}

	auxPowData, err := ParseAuxPowData(auxPowHex, handle.config.MainChain)
	if err != nil {
		response.Error = RPCError{400, err.Error()}
		return
	}

	hashtmp, err := hash.MakeByte32FromHex(hashHex)
	if err != nil {
		response.Error = RPCError{400, err.Error()}
		return
	}
	hashtmp = hashtmp.Reverse()

	job, err := handle.auxJobMaker.FindAuxJob(hashtmp)
	if err != nil {
		response.Error = RPCError{400, err.Error()}
		return
	}

	count := 0
	for index, extAuxPow := range job.AuxPows {
		if glog.V(3) {
			glog.Info("[SubmitAuxBlock] <", handle.auxJobMaker.chains[index].Name, "> blockHash: ",
				auxPowData.blockHash.Hex(), "; auxTarget: ", extAuxPow.Target.Hex())
		}

		// target reached
		if auxPowData.blockHash.Hex() <= extAuxPow.Target.Hex() {

			go func(index int, auxPowData AuxPowData, extAuxPow AuxPowInfo) {
				chain := handle.auxJobMaker.chains[index]
				auxPowData.ExpandingBlockchainBranch(extAuxPow.BlockchainBranch)
				auxPowHex := auxPowData.ToHex()

//slice is a reference to the original string
//Modifications to the string in the slice will directly change the value in chain.SubmitAuxBlock.Params
//So here is a copy
				params := DeepCopy(chain.SubmitAuxBlock.Params)

				if paramsArr, ok := params.([]interface{}); ok { // JSON-RPC 1.0 param array
					for i := range paramsArr {
						if str, ok := paramsArr[i].(string); ok {
							str = strings.Replace(str, "{hash-hex}", extAuxPow.Hash.HexReverse(), -1)
							str = strings.Replace(str, "{aux-pow-hex}", auxPowHex, -1)
							paramsArr[i] = str
						}
					}

				} else if paramsMap, ok := params.(map[string]interface{}); ok { // JSON-RPC 2.0 param object
					for k := range paramsMap {
						if str, ok := paramsMap[k].(string); ok {
							str = strings.Replace(str, "{hash-hex}", extAuxPow.Hash.HexReverse(), -1)
							str = strings.Replace(str, "{aux-pow-hex}", auxPowHex, -1)
							paramsMap[k] = str
						}
					}
				}

				responseJSON, _ := RPCCall(chain.RPCServer, chain.SubmitAuxBlock.Method, params)

				var submitauxblockinfo SubmitAuxBlockInfo
				response, err := ParseRPCResponse(responseJSON)
				if response.Error != nil {
                    submitauxblockinfo.IsSubmitSuccess = false;
				} else {
                    submitauxblockinfo.IsSubmitSuccess = true;
				}
				submitauxblockinfo.AuxBlockTableName = handle.auxJobMaker.chains[index].AuxTableName
				if handle.config.MainChain == "LTC" {
					submitauxblockinfo.ParentChainBllockHash = HexToString(ArrayReverse(DoubleSHA256(auxPowData.parentBlock)))
					submitauxblockinfo.AuxChainBlockHash = extAuxPow.Hash.HexReverse()
				} else {
					submitauxblockinfo.ParentChainBllockHash = auxPowData.blockHash.Hex()
					submitauxblockinfo.AuxChainBlockHash = extAuxPow.Hash.Hex()
				}

				submitauxblockinfo.ChainName = handle.auxJobMaker.chains[index].Name

				submitauxblockinfo.AuxPow = auxPowHex
				submitauxblockinfo.SubmitResponse = string(responseJSON)
				submitauxblockinfo.CurrentTime = time.Now().Format("2006-01-02 15:04:05") 

				if ok = handle.dbhandle.InsertAuxBlock(submitauxblockinfo); !ok {
					glog.Warning("Insert AuxBlock to db failed!")
				}

				glog.Info(
					"[SubmitAuxBlock] <", handle.auxJobMaker.chains[index].Name, "> ",
					", height: ", extAuxPow.Height,
					", hash: ", submitauxblockinfo.AuxChainBlockHash,
					", parentBlockHash: ", submitauxblockinfo.ParentChainBllockHash,
					", target: ", extAuxPow.Target.Hex(),
					", response: ", string(responseJSON),
					", errmsg: ", err)

			}(index, *auxPowData, extAuxPow)

			count++
		}
	}

	if count < 1 {
		glog.Warning("[SubmitAuxBlock] high hash! blockHash: ", auxPowData.blockHash.Hex(), "; maxTarget: ", job.MaxTarget.Hex())
		response.Error = RPCError{400, "high-hash"}
		return
	}

	response.Result = true
	return
}

func runHTTPServer(config ProxyRPCServer, auxJobMaker *AuxJobMaker) {

	handle := NewProxyRPCHandle(config, auxJobMaker)
	// HTTP listening
	glog.Info("Listen HTTP ", config.ListenAddr)
	err := http.ListenAndServe(config.ListenAddr, handle)

	if err != nil {
		glog.Fatal("HTTP Listen Failed: ", err)
		return
	}
}
