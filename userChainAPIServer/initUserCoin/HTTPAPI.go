package initusercoin

// #cgo CXXFLAGS: -std=c++11
// #include "UserListJSON.h"
import "C"

import (
	"net/http"
	"strconv"
	"unsafe"

	"github.com/golang/glog"
)

// HTTPRequestHandle HTTP request handler
type HTTPRequestHandle func(http.ResponseWriter, *http.Request)

// 启动 API Server
func runAPIServer() {
	defer waitGroup.Done()

	// HTTP listening
	glog.Info("Listen HTTP ", configData.ListenAddr)

	http.HandleFunc("/", getUserIDList)

	err := http.ListenAndServe(configData.ListenAddr, nil)

	if err != nil {
		glog.Fatal("HTTP Listen Failed: ", err)
		return
	}
}

// getUserIDList Get a list of sub-accounts
func getUserIDList(w http.ResponseWriter, req *http.Request) {
	coin := req.FormValue("coin")
	lastIDStr := req.FormValue("last_id")
	lastID, _ := strconv.Atoi(lastIDStr)

	coinC := C.CString(coin)
	json := C.GoString(C.getUserListJson(C.int(lastID), coinC))
	C.free(unsafe.Pointer(coinC))
	w.Write([]byte(json))
}

// GetUserUpdateTime Get the user's update time (i.e. when the list was entered)
func GetUserUpdateTime(puname string, coin string) int64 {
	punameC := C.CString(puname)
	coinC := C.CString(coin)
	defer C.free(unsafe.Pointer(punameC))
	defer C.free(unsafe.Pointer(coinC))
	return int64(C.getUserUpdateTime(punameC, coinC))
}

// GetSafetyPeriod Get the security period of the user update (during the security period, the sub-account may not have entered the sserver's cache)
func GetSafetyPeriod() int64 {
	return int64(configData.IntervalSeconds * 15 / 10)
}
