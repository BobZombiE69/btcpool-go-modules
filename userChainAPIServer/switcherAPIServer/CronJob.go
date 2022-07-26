package switcherapiserver

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/golang/glog"
)

// UserCoinMapData The data field data structure of the user currency list interface response
type UserCoinMapData struct {
	UserCoin map[string]string `json:"user_coin"`
	NowDate  int64             `json:"now_date"`
}

// UserCoinMapResponse The data structure of the user currency list interface response
type UserCoinMapResponse struct {
	ErrNo  int             `json:"err_no"`
	ErrMsg string          `json:"err_msg"`
	Data   UserCoinMapData `json:"data"`
}

// RunCronJob Run timed detection tasks
func RunCronJob() {
	defer waitGroup.Done()

	// The last time the interface was requested
	var lastRequestDate int64

	for true {
		// Put hibernation at the beginning to prevent Too new user from being reported as soon as it starts
		time.Sleep(time.Duration(configData.CronIntervalSeconds) * time.Second)

		// perform action
		// Defined in the function, so that when it fails, you can simply return and go to sleep
		func() {

			url := configData.UserCoinMapURL
			// If the interface was requested last time, append the time of the last request to the url
			if lastRequestDate > 0 {
				// configData.CronIntervalSeconds is subtracted to prevent race conditions.
				// For example, after the last pull, there is another currency switch within the same second. If it is not subtracted, the switch message may be missed.
				url += "?last_date=" + strconv.FormatInt(lastRequestDate-int64(configData.CronIntervalSeconds), 10)
			}
			glog.Info("HTTP GET ", url)
			response, err := http.Get(url)

			if err != nil {
				glog.Error("HTTP Request Failed: ", err)
				return
			}

			body, err := ioutil.ReadAll(response.Body)

			if err != nil {
				glog.Error("HTTP Fetch Body Failed: ", err)
				return
			}

			userCoinMapResponse := new(UserCoinMapResponse)

			err = json.Unmarshal(body, userCoinMapResponse)

			if err != nil {
				glog.Error("Parse Result Failed: ", err, "; ", string(body))
				return
			}

			if userCoinMapResponse.ErrNo != 0 {
				glog.Error("API Returned a Error: ", string(body))
				return
			}

			// Record the time of this request
			lastRequestDate = userCoinMapResponse.Data.NowDate

			glog.Info("HTTP GET Success. TimeStamp: ", userCoinMapResponse.Data.NowDate, "; UserCoin Num: ", len(userCoinMapResponse.Data.UserCoin))

			// Traverse the user currency list
			for puname, coin := range userCoinMapResponse.Data.UserCoin {
				oldCoin, err := changeMiningCoin(puname, coin)

				if err != nil {
					glog.Info(err.ErrMsg, ": ", puname, ": ", oldCoin, " -> ", coin)
				} else {
					glog.Info("success: ", puname, ": ", oldCoin, " -> ", coin)
				}
			}
		}()
	}
}
