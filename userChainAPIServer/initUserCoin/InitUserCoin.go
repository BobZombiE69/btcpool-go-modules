package initusercoin

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/golang/glog"
	"github.com/samuel/go-zookeeper/zk"
)

// #cgo CXXFLAGS: -std=c++11
// #include "UserListJSON.h"
import "C"

// UserIDMapResponse The data structure of the user id list interface response
type UserIDMapResponse struct {
	ErrNo  int            `json:"err_no"`
	ErrMsg string         `json:"err_msg"`
	Data   map[string]int `json:"data"`
}

// UserIDMapEmptyResponse The response of the user id list interface when the number of users is 0
type UserIDMapEmptyResponse struct {
	ErrNo  int           `json:"err_no"`
	ErrMsg string        `json:"err_msg"`
	Data   []interface{} `json:"data"`
}

// InitUserCoin Pull the user id list to initialize the user currency record
func InitUserCoin(coin string, url string) {
	defer waitGroup.Done()

	// max puid of last request
	lastPUID := 0

	for {
		// perform action
		// Defined in the function, so that when it fails, you can simply return and go to sleep
		func() {
			urlWithLastID := url + "?last_id=" + strconv.Itoa(lastPUID)

			glog.Info("HTTP GET ", urlWithLastID)
			response, err := http.Get(urlWithLastID)

			if err != nil {
				glog.Error("HTTP Request Failed: ", err)
				return
			}

			body, err := ioutil.ReadAll(response.Body)

			if err != nil {
				glog.Error("HTTP Fetch Body Failed: ", err)
				return
			}

			userIDMapResponse := new(UserIDMapResponse)
			err = json.Unmarshal(body, userIDMapResponse)

			if err != nil {
				// When the user id interface returns 0 users, the data type of the data field will change from object to array, which needs to be parsed with another struct
				userIDMapEmptyResponse := new(UserIDMapEmptyResponse)
				err = json.Unmarshal(body, userIDMapEmptyResponse)

				if err != nil {
					glog.Error("Parse Result Failed: ", err, "; ", string(body))
					return
				}

				glog.Info("Finish: ", coin, "; No New User", "; ", url)
				return
			}

			if userIDMapResponse.ErrNo != 0 {
				glog.Error("API Returned a Error: ", string(body))
				return
			}

			glog.Info("HTTP GET Success. User Num: ", len(userIDMapResponse.Data))

			// Traverse the user currency list
			for puname, puid := range userIDMapResponse.Data {
				if strings.Contains(puname, "_") {
					// remove coin postfix of puname
					puname = puname[0:strings.LastIndex(puname, "_")]
				}

				err := setMiningCoin(puname, coin)

				if err != nil {
					glog.Info(err.ErrMsg, ": ", puname, ": ", coin)

					if err != APIErrRecordExists {
						continue
					}
				} else {
					glog.Info("success: ", puname, " (", puid, "): ", coin)
				}

				if puid > lastPUID {
					lastPUID = puid
				}

				punameC := C.CString(puname)
				coinC := C.CString(coin)
				C.addUser(C.int(puid), punameC, coinC)
				C.free(unsafe.Pointer(punameC))
				C.free(unsafe.Pointer(coinC))
			}

			glog.Info("Finish: ", coin, "; User Num: ", len(userIDMapResponse.Data), "; ", url)
		}()

		// hibernate
		time.Sleep(time.Duration(configData.IntervalSeconds) * time.Second)
	}
}

func setMiningCoin(puname string, coin string) (apiErr *APIError) {

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

	for availableCoin := range configData.UserListAPI {
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
	} else if len(configData.ZKUserCaseInsensitiveIndex) > 0 {
		// stratum server is case sensitive for sub-account names
		// and ZKUserCaseInsensitiveIndex is not disabled (not empty)
		// Write case-insensitive username index
		zkIndexPath := configData.ZKUserCaseInsensitiveIndex + strings.ToLower(puname)
		exists, _, err := zookeeperConn.Exists(zkIndexPath)
		if err != nil {
			glog.Error("zk.Exists(", zkIndexPath, ",", puname, ") Failed: ", err)
		}
		if !exists {
			_, err = zookeeperConn.Create(zkIndexPath, []byte(puname), 0, zk.WorldACL(zk.PermAll))
			if err != nil {
				glog.Error("zk.Create(", zkIndexPath, ",", puname, ") Failed: ", err)
			}
		}
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
		// already exists, skip
		apiErr = APIErrRecordExists
		return

	}

	// does not exist, create
	_, err = zookeeperConn.Create(zkPath, []byte(coin), 0, zk.WorldACL(zk.PermAll))

	if err != nil {
		glog.Error("zk.Create(", zkPath, ",", coin, ") Failed: ", err)
		apiErr = APIErrWriteRecordFailed
		return
	}

	apiErr = nil
	return
}
