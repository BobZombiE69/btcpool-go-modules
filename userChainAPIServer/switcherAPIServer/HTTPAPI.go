package switcherapiserver

import (
	"crypto/subtle"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	initusercoin "github.com/BobZombiE69/btcpool-go-modules/userChainAPIServer/initUserCoin"
	"github.com/golang/glog"
	"github.com/samuel/go-zookeeper/zk"
)

// SwitchUserCoins User and currency to switch
type SwitchUserCoins struct {
	Coin    string   `json:"coin"`
	PUNames []string `json:"punames"`
}

// SwitchMultiUserRequest Multi-user handover request data structure
type SwitchMultiUserRequest struct {
	UserCoins []SwitchUserCoins `json:"usercoins"`
}

// APIResponse API response data structure
type APIResponse struct {
	ErrNo   int    `json:"err_no"`
	ErrMsg  string `json:"err_msg"`
	Success bool   `json:"success"`
}

// SubPoolUpdate Subpool update information
type SubPoolUpdate struct {
	Coin         string `json:"coin"`
	SubPoolName  string `json:"subpool_name"`
	CoinbaseInfo string `json:"coinbase_info"`
	PayoutAddr   string `json:"payout_addr"`
}

// SubPoolCoinbase Subpool Coinbase Information
type SubPoolCoinbase struct {
	Success     bool   `json:"success"`
	ErrNo       int    `json:"err_no"`
	ErrMsg      string `json:"err_msg"`
	SubPoolName string `json:"subpool_name"`
	Old         struct {
		CoinbaseInfo string `json:"coinbase_info"`
		PayoutAddr   string `json:"payout_addr"`
	} `json:"old"`
}

// SubPoolUpdateAck Subpool update response
type SubPoolUpdateAck struct {
	SubPoolCoinbase
	New struct {
		CoinbaseInfo string `json:"coinbase_info"`
		PayoutAddr   string `json:"payout_addr"`
	} `json:"new"`
}

// SubPoolUpdateAckInner Subpool update response (non-public)
type SubPoolUpdateAckInner struct {
	SubPoolUpdateAck
	Host struct {
		HostName string `json:"hostname"`
	} `json:"host"`
}

// HTTPRequestHandle HTTP request handler
type HTTPRequestHandle func(http.ResponseWriter, *http.Request)

// start up API Server
func runAPIServer() {
	defer waitGroup.Done()

	// HTTP listening
	glog.Info("Listen HTTP ", configData.ListenAddr)

	http.HandleFunc("/switch", basicAuth(switchHandle))

	http.HandleFunc("/switch/multi-user", basicAuth(switchMultiUserHandle))
	http.HandleFunc("/switch-multi-user", basicAuth(switchMultiUserHandle))

	http.HandleFunc("/subpool/get-coinbase", basicAuth(getCoinbaseHandle))
	http.HandleFunc("/subpool-get-coinbase", basicAuth(getCoinbaseHandle))

	http.HandleFunc("/subpool/update-coinbase", basicAuth(updateCoinbaseHandle))
	http.HandleFunc("/subpool-update-coinbase", basicAuth(updateCoinbaseHandle))

	// The listener will be done in initUserCoin/HTTPAPI.go
	/*err := http.ListenAndServe(configData.ListenAddr, nil)

	if err != nil {
		glog.Fatal("HTTP Listen Failed: ", err)
		return
	}*/
}

// basicAuth Perform Basic authentication
func basicAuth(f HTTPRequestHandle) HTTPRequestHandle {
	return func(w http.ResponseWriter, r *http.Request) {
		apiUser := []byte(configData.APIUser)
		apiPasswd := []byte(configData.APIPassword)

		user, passwd, ok := r.BasicAuth()

		// Check if the username and password are correct
		if ok && subtle.ConstantTimeCompare(apiUser, []byte(user)) == 1 && subtle.ConstantTimeCompare(apiPasswd, []byte(passwd)) == 1 {
			// execute the decorated function
			f(w, r)
			return
		}

		// Authentication failed with 401 Unauthorized
		// Restricted can be changed to other values
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		// 401 status code
		w.WriteHeader(http.StatusUnauthorized)
		// 401 page
		w.Write([]byte(`<h1>401 - Unauthorized</h1>`))
	}
}

// getCoinbaseHandle Get sub-pool coinbase information
func getCoinbaseHandle(w http.ResponseWriter, req *http.Request) {
	if len(configData.ZKSubPoolUpdateBaseDir) == 0 {
		writeError(w, 403, "API disabled")
		return
	}

	requestJSON, err := ioutil.ReadAll(req.Body)

	if err != nil {
		glog.Warning(err, ": ", req.RequestURI)
		writeError(w, 500, err.Error())
		return
	}

	var reqData SubPoolUpdate
	err = json.Unmarshal(requestJSON, &reqData)

	if err != nil {
		glog.Info(err, ": ", req.RequestURI)
		writeError(w, 400, "wrong JSON, "+err.Error())
		return
	}

	if len(reqData.Coin) < 1 {
		writeError(w, 400, "coin cannot be empty")
		return
	}
	if len(reqData.SubPoolName) < 1 {
		writeError(w, 400, "subpool_name cannot be empty")
		return
	}

	glog.Info("[subpool-get] Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName)

	reqNode := configData.ZKSubPoolUpdateBaseDir + reqData.Coin + "/" + reqData.SubPoolName
	ackNode := reqNode + "/ack"

	reqByte, stat, err := zookeeperConn.Get(reqNode)
	if err != nil {
		glog.Warning("[subpool-get] zk path '", reqNode, "' doesn't exists",
			" Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName)
		writeError(w, 404, "subpool '"+reqData.SubPoolName+"' does not exist")
		return
	}

	exists, _, ack, err := zookeeperConn.ExistsW(ackNode)
	if err != nil || !exists {
		glog.Warning("[subpool-get] zk path '", ackNode, "' doesn't exists",
			" Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName)
		writeError(w, 503, "jobmaker cannot ACK the request")
		return
	}

	_, err = zookeeperConn.Set(reqNode, reqByte, stat.Version)
	if err != nil {
		glog.Warning("[subpool-get] data has been updated at query time! ", err.Error(),
			" Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName)
		writeError(w, 500, "data has been updated at query time")
		return
	}

	select {
	case <-ack:
		ackJSON, _, err := zookeeperConn.Get(ackNode)
		if err != nil {
			glog.Warning("[subpool-get] get ACK failed, ", err.Error(),
				" Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName)
			writeError(w, 500, "cannot get ACK from zookeeper")
			return
		}

		var ackData SubPoolUpdateAckInner
		err = json.Unmarshal(ackJSON, &ackData)
		if err != nil {
			glog.Warning("[subpool-get] parse ACK failed, ", err.Error(),
				" Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName)
			writeError(w, 500, "cannot parse ACK in zookeeper")
			return
		}

		if !ackData.Success && ackData.ErrMsg == "empty request" {
			ackData.Success = true
			ackData.ErrMsg = "success"
		}

		glog.Info("[subpool-get] Response: ", ackData.ErrMsg, ", Host: ", ackData.Host.HostName,
			", Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName,
			", Old: ", ackData.Old)

		ackByte, _ := json.Marshal(ackData.SubPoolCoinbase)
		w.Write(ackByte)
		return

	case <-time.After(time.Duration(configData.ZKSubPoolUpdateAckTimeout) * time.Second):
		glog.Warning("[subpool-get] ", "timeout when waiting ACK!",
			" Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName)
		writeError(w, 504, "timeout when waiting ACK")
		return
	}
}

// updateCoinbaseHandle Update subpool coinbase information
func updateCoinbaseHandle(w http.ResponseWriter, req *http.Request) {
	if len(configData.ZKSubPoolUpdateBaseDir) == 0 {
		writeError(w, 403, "API disabled")
		return
	}

	requestJSON, err := ioutil.ReadAll(req.Body)

	if err != nil {
		glog.Warning(err, ": ", req.RequestURI)
		writeError(w, 500, err.Error())
		return
	}

	var reqData SubPoolUpdate
	err = json.Unmarshal(requestJSON, &reqData)

	if err != nil {
		glog.Info(err, ": ", req.RequestURI)
		writeError(w, 400, "wrong JSON, "+err.Error())
		return
	}

	if len(reqData.Coin) < 1 {
		writeError(w, 400, "coin cannot be empty")
		return
	}
	if len(reqData.SubPoolName) < 1 {
		writeError(w, 400, "subpool_name cannot be empty")
		return
	}
	if len(reqData.PayoutAddr) < 1 {
		writeError(w, 400, "payout_addr cannot be empty")
		return
	}

	glog.Info("[subpool-update] Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName,
		", CoinbaseInfo: ", reqData.CoinbaseInfo, ", PayoutAddr: ", reqData.PayoutAddr)

	reqNode := configData.ZKSubPoolUpdateBaseDir + reqData.Coin + "/" + reqData.SubPoolName
	ackNode := reqNode + "/ack"

	exists, _, err := zookeeperConn.Exists(reqNode)
	if err != nil || !exists {
		glog.Warning("[subpool-update] zk path '", reqNode, "' doesn't exists",
			" Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName)
		writeError(w, 404, "subpool '"+reqData.SubPoolName+"' does not exist")
		return
	}

	exists, _, ack, err := zookeeperConn.ExistsW(ackNode)
	if err != nil || !exists {
		glog.Warning("[subpool-update] zk path '", ackNode, "' doesn't exists",
			" Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName)
		writeError(w, 503, "jobmaker cannot ACK the request")
		return
	}

	reqByte, _ := json.Marshal(reqData)
	_, err = zookeeperConn.Set(reqNode, reqByte, -1)
	if err != nil {
		glog.Warning("[subpool-update] set zk path '", reqNode, "' failed! ", err.Error(),
			" Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName)
		writeError(w, 500, "write data node failed")
		return
	}

	select {
	case <-ack:
		ackJSON, _, err := zookeeperConn.Get(ackNode)
		if err != nil {
			glog.Warning("[subpool-update] get ACK failed, ", err.Error(),
				" Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName)
			writeError(w, 500, "cannot get ACK from zookeeper")
			return
		}

		var ackData SubPoolUpdateAckInner
		err = json.Unmarshal(ackJSON, &ackData)
		if err != nil {
			glog.Warning("[subpool-update] parse ACK failed, ", err.Error(),
				" Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName)
			writeError(w, 500, "cannot parse ACK in zookeeper")
			return
		}

		if !ackData.Success && ackData.ErrNo == 0 {
			ackData.ErrNo = 500
		}

		glog.Info("[subpool-update] Response: ", ackData.ErrMsg, ", Host: ", ackData.Host.HostName,
			", Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName,
			", Old: ", ackData.Old, ", New: ", ackData.New)

		ackByte, _ := json.Marshal(ackData.SubPoolUpdateAck)
		w.Write(ackByte)
		return

	case <-time.After(time.Duration(configData.ZKSubPoolUpdateAckTimeout) * time.Second):
		glog.Warning("[subpool-update] ", "timeout when waiting ACK!",
			" Coin: ", reqData.Coin, ", SubPool: ", reqData.SubPoolName)
		writeError(w, 504, "timeout when waiting ACK")
		return
	}
}

// switchHandle Handling currency switching requests
func switchHandle(w http.ResponseWriter, req *http.Request) {
	puname := req.FormValue("puname")
	coin := req.FormValue("coin")

	oldCoin, err := changeMiningCoin(puname, coin)

	if err != nil {
		glog.Info(err, ": ", req.RequestURI)
		writeError(w, err.ErrNo, err.ErrMsg)
		return
	}

	glog.Info("[single-switch] ", puname, ": ", oldCoin, " -> ", coin)
	writeSuccess(w)
}

// switchMultiUserHandle Handling multi-user currency switching requests
func switchMultiUserHandle(w http.ResponseWriter, req *http.Request) {
	var reqData SwitchMultiUserRequest

	requestJSON, err := ioutil.ReadAll(req.Body)

	if err != nil {
		glog.Warning(err, ": ", req.RequestURI)
		writeError(w, 500, err.Error())
		return
	}

	err = json.Unmarshal(requestJSON, &reqData)

	if err != nil {
		glog.Info(err, ": ", req.RequestURI)
		writeError(w, 400, err.Error())
		return
	}

	if len(reqData.UserCoins) == 0 {
		glog.Info(APIErrUserCoinsEmpty.ErrMsg, ": ", req.RequestURI)
		writeError(w, APIErrUserCoinsEmpty.ErrNo, APIErrUserCoinsEmpty.ErrMsg)
		return
	}

	for _, usercoin := range reqData.UserCoins {
		coin := usercoin.Coin

		for _, puname := range usercoin.PUNames {
			oldCoin, err := changeMiningCoin(puname, coin)

			if err != nil {
				glog.Info(err, ": ", req.RequestURI, " {puname=", puname, ", coin=", coin, "}")
				writeError(w, err.ErrNo, err.ErrMsg)
				return
			}

			glog.Info("[multi-switch] ", puname, ": ", oldCoin, " -> ", coin)
		}
	}

	writeSuccess(w)
}

func writeSuccess(w http.ResponseWriter) {
	response := APIResponse{0, "", true}
	responseJSON, _ := json.Marshal(response)

	w.Write(responseJSON)
}

func writeError(w http.ResponseWriter, errNo int, errMsg string) {
	response := APIResponse{errNo, errMsg, false}
	responseJSON, _ := json.Marshal(response)

	w.Write(responseJSON)
}

func changeMiningCoin(puname string, coin string) (oldCoin string, apiErr *APIError) {
	oldCoin = ""

	if len(puname) < 1 {
		apiErr = APIErrPunameIsEmpty
		return
	}

	if strings.Contains(puname, "/") {
		apiErr = APIErrPunameInvalid
		return
	}

	if len(coin) < 1 {
		apiErr = APIErrCoinIsEmpty
		return
	}

	// Check if currency exists
	exists := false

	for _, availableCoin := range configData.AvailableCoins {
		if availableCoin == coin {
			exists = true
			break
		}
	}

	if !exists {
		apiErr = APIErrCoinIsInexistent
		return
	}

	if configData.StratumServerCaseInsensitive {
		// stratum server is not case sensitive to sub-account names
		// Simply convert the sub-account name to lowercase
		puname = strings.ToLower(puname)
	}

	// stratumSwitcher monitor key
	zkPath := configData.ZKSwitcherWatchDir + puname

	// see if the key exists
	exists, _, err := zookeeperConn.Exists(zkPath)

	if err != nil {
		glog.Error("zk.Exists(", zkPath, ") Failed: ", err)
		apiErr = APIErrReadRecordFailed
		return
	}

	if exists {
		// Read zookeeper to see what the original value is
		oldCoinData, _, err := zookeeperConn.Get(zkPath)

		if err != nil {
			glog.Error("zk.Get(", zkPath, ") Failed: ", err)
			apiErr = APIErrReadRecordFailed
			return
		}

		oldCoin = string(oldCoinData)

		// No change
		// No change no longer returns an error, so that if stratumSwitcher missed the previous switch message, it can receive another switch message to complete the switch
		// In stratumSwitcher, if the currency does not change, the switch will not happen
		/*if oldCoin == coin {
			apiErr = APIErrCoinNoChange
			return
		}*/

		// Check the update time of the sub-account name. If the sub-account name has just been created, it will be written with a delay of 15 seconds
		userUpdateTime := initusercoin.GetUserUpdateTime(puname, coin)
		safetyPeriod := initusercoin.GetSafetyPeriod()
		nowTime := time.Now().Unix()

		if userUpdateTime != 0 && nowTime-userUpdateTime >= safetyPeriod {
			// write new value
			_, err = zookeeperConn.Set(zkPath, []byte(coin), -1)

			if err != nil {
				glog.Error("zk.Set(", zkPath, ",", coin, ") Failed: ", err)
				apiErr = APIErrWriteRecordFailed
				return
			}
		} else {
			if userUpdateTime <= 0 {
				userUpdateTime = nowTime
			}
			sleepTime := safetyPeriod - (nowTime - userUpdateTime)
			glog.Info("Too new puname ", puname, ", delay ", sleepTime, "s")

			go func() {
				time.Sleep(time.Duration(sleepTime) * time.Second)

				// write new value
				_, err = zookeeperConn.Set(zkPath, []byte(coin), -1)

				if err != nil {
					glog.Error("zk.Set(", zkPath, ",", coin, ") Failed: ", err)
				}
			}()
		}

	} else {
		// does not exist, create it directly
		_, err = zookeeperConn.Create(zkPath, []byte(coin), 0, zk.WorldACL(zk.PermAll))

		if err != nil {
			glog.Error("zk.Create(", zkPath, ",", coin, ") Failed: ", err)
			apiErr = APIErrWriteRecordFailed
			return
		}
	}

	apiErr = nil
	return
}
