package switcherapiserver

import (
	"encoding/json"
	"io/ioutil"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/samuel/go-zookeeper/zk"
)

// Zookeeper connection timeout
const zookeeperConnTimeout = 5

// ConfigData Configuration Data
type ConfigData struct {
	// Whether to enable API Server
	EnableAPIServer bool
	// API username
	APIUser string
	// API password
	APIPassword string
	// API Server The listening IP:port
	ListenAddr string

	// AvailableCoins Available currencies, like {"btc", "bcc", ...}
	AvailableCoins []string

	// Zookeeper cluster IP:port list
	ZKBroker []string
	// ZKSwitcherWatchDir Zookeeper path monitored by Switcher, ending with a slash
	ZKSwitcherWatchDir string

	// Whether to enable scheduled detection tasks
	EnableCronJob bool
	// Timing detection interval time
	CronIntervalSeconds int
	// User: URL of currency correspondence table
	UserCoinMapURL string
	// The mining server is not case sensitive to the sub-account name, in this case, it will always write the sub-account name in lowercase
	StratumServerCaseInsensitive bool
	//The zookeeper root directory for sub-pool updates (note that the currency and sub-pool name should not be included), ending with a slash
	ZKSubPoolUpdateBaseDir string
	// The response timeout time of the jobmaker when the subpool is updated. If the jobmaker does not respond within this time, the API returns an error
	ZKSubPoolUpdateAckTimeout int
}

// zookeeperConn Zookeeper connection object
var zookeeperConn *zk.Conn

// Configuration Data
var configData *ConfigData

// Used to wait for the goroutine to finish
var waitGroup sync.WaitGroup

// Main function
func Main(configFilePath string) {
	// read configuration file
	configJSON, err := ioutil.ReadFile(configFilePath)

	if err != nil {
		glog.Fatal("read config failed: ", err)
		return
	}

	configData = new(ConfigData)
	err = json.Unmarshal(configJSON, configData)

	if err != nil {
		glog.Fatal("parse config failed: ", err)
		return
	}

	// If the zookeeper path does not end with "/", add
	if configData.ZKSwitcherWatchDir[len(configData.ZKSwitcherWatchDir)-1] != '/' {
		configData.ZKSwitcherWatchDir += "/"
	}
	if len(configData.ZKSubPoolUpdateBaseDir) > 0 && configData.ZKSubPoolUpdateBaseDir[len(configData.ZKSubPoolUpdateBaseDir)-1] != '/' {
		configData.ZKSubPoolUpdateBaseDir += "/"
	}

	// Establish a connection to the Zookeeper cluster
	conn, _, err := zk.Connect(configData.ZKBroker, time.Duration(zookeeperConnTimeout)*time.Second)

	if err != nil {
		glog.Fatal("Connect Zookeeper Failed: ", err)
		return
	}

	zookeeperConn = conn

	// Check and create Zookeeper paths used by StratumSwitcher
	err = createZookeeperPath(configData.ZKSwitcherWatchDir)

	if err != nil {
		glog.Fatal("Create Zookeeper Path Failed: ", err)
		return
	}

	if configData.EnableAPIServer {
		waitGroup.Add(1)
		go runAPIServer()
	}

	if configData.EnableCronJob {
		waitGroup.Add(1)
		go RunCronJob()
	}

	waitGroup.Wait()
}
