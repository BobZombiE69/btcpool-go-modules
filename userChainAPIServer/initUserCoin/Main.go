package initusercoin

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

// AutoRegAPIConfig User auto-registration API definition
type AutoRegAPIConfig struct {
	IntervalSeconds time.Duration
	URL             string
	User            string
	Password        string
	DefaultCoin     string
	PostData        map[string]string
}

// ConfigData Configuration Data
type ConfigData struct {
	// UserListAPI The list of users corresponding to the currency, in the form of {"btc":"url", "bcc":"url"}
	UserListAPI map[string]string
	// IntervalSeconds The time between each pull
	IntervalSeconds uint

	// Zookeeper cluster IP:port list
	ZKBroker []string
	// ZKSwitcherWatchDir Zookeeper path monitored by Switcher, ending with a slash
	ZKSwitcherWatchDir string

	// EnableUserAutoReg Enable user auto-registration
	EnableUserAutoReg bool
	// ZKAutoRegWatchDir The zookeeper monitoring address automatically registered by the user, ending with a slash
	ZKAutoRegWatchDir string
	// UserAutoRegAPI User auto-registration API
	UserAutoRegAPI AutoRegAPIConfig
	// StratumServerCaseInsensitive The mining server is not case sensitive to the sub-account name, in this case, it will always write the sub-account name in lowercase
	StratumServerCaseInsensitive bool
	// ZKUserCaseInsensitiveIndex Case-insensitive subaccount index
	//（Nullable, only if StratumServerCaseInsensitive == false used)
	ZKUserCaseInsensitiveIndex string

	// Whether to enable API Server
	EnableAPIServer bool
	// API Server The listening IP:port
	ListenAddr string
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
	if configData.EnableUserAutoReg && configData.ZKAutoRegWatchDir[len(configData.ZKAutoRegWatchDir)-1] != '/' {
		configData.ZKAutoRegWatchDir += "/"
	}
	if !configData.StratumServerCaseInsensitive &&
		len(configData.ZKUserCaseInsensitiveIndex) > 0 &&
		configData.ZKUserCaseInsensitiveIndex[len(configData.ZKUserCaseInsensitiveIndex)-1] != '/' {
		configData.ZKUserCaseInsensitiveIndex += "/"
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

	if configData.EnableUserAutoReg {
		err = createZookeeperPath(configData.ZKAutoRegWatchDir)

		if err != nil {
			glog.Fatal("Create Zookeeper Path Failed: ", err)
			return
		}
	}

	if !configData.StratumServerCaseInsensitive && len(configData.ZKUserCaseInsensitiveIndex) > 0 {
		err = createZookeeperPath(configData.ZKUserCaseInsensitiveIndex)

		if err != nil {
			glog.Fatal("Create Zookeeper Path Failed: ", err)
			return
		}
	}

	// Start the currency initialization task
	for coin, url := range configData.UserListAPI {
		waitGroup.Add(1)
		go InitUserCoin(coin, url)
	}

	// Start automatic registration
	if configData.EnableUserAutoReg {
		waitGroup.Add(1)
		go RunUserAutoReg(configData)
	}

	// Start the Subaccount List API
	if configData.EnableAPIServer {
		waitGroup.Add(1)
		go runAPIServer()
	}

	waitGroup.Wait()

	glog.Info("Init User Coin Finished.")
}
