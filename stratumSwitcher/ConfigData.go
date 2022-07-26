package main

import (
	"encoding/json"
	"io/ioutil"

	"github.com/golang/glog"
)

// ChainType blockchain type
type ChainType uint8

const (
	// ChainTypeBitcoin Bitcoin or similar blockchain
	ChainTypeBitcoin ChainType = iota
	// ChainTypeDecredNormal DCR Normal
	ChainTypeDecredNormal
	// ChainTypeDecredGoMiner DCR GoMiner
	ChainTypeDecredGoMiner
	// ChainTypeEthereum Ethereum or similar blockchain
	ChainTypeEthereum
)

// ToString convert to string
func (chainType ChainType) ToString() string {
	switch chainType {
	case ChainTypeBitcoin:
		return "bitcoin"
	case ChainTypeDecredNormal:
		return "decred-normal"
	case ChainTypeDecredGoMiner:
		return "decred-gominer"
	case ChainTypeEthereum:
		return "ethereum"
	default:
		return "unknown"
	}
}

// ConfigData Configuration Data
type ConfigData struct {
	ServerID                     uint8
	ChainType                    string
	ListenAddr                   string
	StratumServerMap             StratumServerInfoMap
	ZKBroker                     []string
	ZKServerIDAssignDir          string // ends with a slash
	ZKSwitcherWatchDir           string // ends with a slash
	EnableUserAutoReg            bool
	ZKAutoRegWatchDir            string // ends with a slash
	AutoRegMaxWaitUsers          int64
	StratumServerCaseInsensitive bool
	ZKUserCaseInsensitiveIndex   string // ends with a slash
	EnableHTTPDebug              bool
	HTTPDebugListenAddr          string
}

// LoadFromFile Load configuration from file
func (conf *ConfigData) LoadFromFile(file string) (err error) {

	configJSON, err := ioutil.ReadFile(file)

	if err != nil {
		return
	}

	err = json.Unmarshal(configJSON, conf)

	// If the zookeeper path does not end with "/", add
	if conf.ZKServerIDAssignDir[len(conf.ZKServerIDAssignDir)-1] != '/' {
		conf.ZKServerIDAssignDir += "/"
	}
	if conf.ZKSwitcherWatchDir[len(conf.ZKSwitcherWatchDir)-1] != '/' {
		conf.ZKSwitcherWatchDir += "/"
	}
	if conf.ZKAutoRegWatchDir[len(conf.ZKAutoRegWatchDir)-1] != '/' {
		conf.ZKAutoRegWatchDir += "/"
	}
	if !conf.StratumServerCaseInsensitive &&
		len(conf.ZKUserCaseInsensitiveIndex) > 0 &&
		conf.ZKUserCaseInsensitiveIndex[len(conf.ZKUserCaseInsensitiveIndex)-1] != '/' {
		conf.ZKUserCaseInsensitiveIndex += "/"
	}

	// If UserSuffix is ​​empty, set the same as the currency
	for k, v := range conf.StratumServerMap {
		if v.UserSuffix == "" {
			v.UserSuffix = k
			conf.StratumServerMap[k] = v
		}
		glog.Info("Chain: ", k, ", UserSuffix: ", conf.StratumServerMap[k].UserSuffix)
	}

	return
}

// SaveToFile save configuration to file
func (conf *ConfigData) SaveToFile(file string) (err error) {

	configJSON, err := json.Marshal(conf)

	if err != nil {
		return
	}

	err = ioutil.WriteFile(file, configJSON, 0644)
	return
}

// StratumSessionData Stratum会话数据
type StratumSessionData struct {
	// session id
	SessionID uint32
	// The currency mined by the user
	MiningCoin string

	ClientConnFD uintptr
	ServerConnFD uintptr

	StratumSubscribeRequest *JSONRPCRequest
	StratumAuthorizeRequest *JSONRPCRequest

	// Bitcoin AsicBoost mining version mask
	VersionMask uint32 `json:",omitempty"`
}

// RuntimeData runtime data
type RuntimeData struct {
	Action       string
	ServerID     uint8
	SessionDatas []StratumSessionData
}

// LoadFromFile Load configuration from file
func (conf *RuntimeData) LoadFromFile(file string) (err error) {

	configJSON, err := ioutil.ReadFile(file)

	if err != nil {
		return
	}

	err = json.Unmarshal(configJSON, conf)
	return
}

// SaveToFile save configuration to file
func (conf *RuntimeData) SaveToFile(file string) (err error) {

	configJSON, err := json.Marshal(conf)

	if err != nil {
		return
	}

	err = ioutil.WriteFile(file, configJSON, 0644)
	return
}
