package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"strconv"

	"github.com/golang/glog"
)

// RPCResponseKeys The mapping between the key of the RPC response and the key requested by the program
type RPCResponseKeys map[string]string

// RPCCreateAuxBlockResultKeys The key to map the return result of the RPC method createauxblock
type RPCCreateAuxBlockResultKeys struct {
	Hash          string
	ChainID       string
	Bits          string
	Height        string
	PrevBlockHash string
	CoinbaseValue string
	Target        string
}

// RPCCreateAuxBlockInfo Request and response information for the RPC method createauxblock
type RPCCreateAuxBlockInfo struct {
	Method       string
	Params       interface{}
	ResponseKeys RPCCreateAuxBlockResultKeys
}

// RPCSubmitAuxBlockInfo Request and response information for the RPC method submitauxblock
type RPCSubmitAuxBlockInfo struct {
	Method string
	Params interface{}
}

// ChainRPCServer RPC server for merge-mined chains
type ChainRPCServer struct {
	URL    string
	User   string
	Passwd string
}

type DBConnectionInfo struct {
	Host       string
	Port       string
	Username   string
	Password   string
	Dbname     string
}


// ChainRPCInfo RPC information for merged mining coins
type ChainRPCInfo struct {
	ChainID        uint32
	Name           string
	AuxTableName   string
	RPCServer      ChainRPCServer
	CreateAuxBlock RPCCreateAuxBlockInfo
	SubmitAuxBlock RPCSubmitAuxBlockInfo
	SubBlockHashAddress string
	SubBlockHashPort    string
	IsSupportZmq        bool

}

// ProxyRPCServer RPC server information for this proxy
type ProxyRPCServer struct {
	ListenAddr string
	User       string
	Passwd     string
	MainChain  string
	PoolDb     DBConnectionInfo
}

// AuxJobMakerInfo Auxiliary mining task generation configuration
type AuxJobMakerInfo struct {
	CreateAuxBlockIntervalSeconds uint
	AuxPowJobListSize             uint
	MaxJobTarget                  string
	BlockHashPublishPort          string
}

// ConfigData Configuration file data structure
type ConfigData struct {
	RPCServer   ProxyRPCServer
	AuxJobMaker AuxJobMakerInfo
	Chains      []ChainRPCInfo
}

// Check Check the validity of the configuration
func (conf *ConfigData) Check() (err error) {
	if len(conf.RPCServer.User) < 1 {
		return errors.New("RPCServer.User cannot be empty")
	}

	if len(conf.RPCServer.Passwd) < 1 {
		return errors.New("RPCServer.Passwd cannot be empty")
	}

	if len(conf.RPCServer.ListenAddr) < 1 {
		return errors.New("RPCServer.ListenAddr cannot be empty")
	}

	if len(conf.RPCServer.PoolDb.Host) < 1 {
		return errors.New("RPCServer.PoolDb.Host cannot be empty")
	}

	if len(conf.RPCServer.PoolDb.Port) < 1 {
		return errors.New("RPCServer.PoolDb.Port cannot be empty")
	}

	if len(conf.RPCServer.PoolDb.Username) < 1 {
		return errors.New("RPCServer.PoolDb.Username cannot be empty")
	}

	if len(conf.RPCServer.PoolDb.Password) < 1 {
		return errors.New("RPCServer.PoolDb.Password cannot be empty")
	}

	if len(conf.RPCServer.PoolDb.Dbname) < 1 {
		return errors.New("RPCServer.PoolDb.Dbname cannot be empty")
	}

	if len(conf.Chains) < 1 {
		return errors.New("Chains cannot be empty")
	}

	// Check each Chain
	for index, chain := range conf.Chains {
		if len(chain.Name) < 1 {
			return errors.New("Chains[" + strconv.Itoa(index) + "].Name cannot be empty")
		}
		
		if len(chain.AuxTableName) < 1 {
			return errors.New("Chains[" + strconv.Itoa(index) + "].AuxTableName cannot be empty")
		}


		if len(chain.RPCServer.URL) < 1 {
			return errors.New("Chains[" + strconv.Itoa(index) + "].RPCServer.URL cannot be empty")
		}

		if len(chain.CreateAuxBlock.Method) < 1 {
			return errors.New("Chains[" + strconv.Itoa(index) + "].CreateAuxBlock.Method cannot be empty")
		}

		if len(chain.CreateAuxBlock.ResponseKeys.Hash) < 1 {
			return errors.New("Chains[" + strconv.Itoa(index) + "].CreateAuxBlock.ResponseKeys.Hash cannot be empty")
		}

		if len(chain.CreateAuxBlock.ResponseKeys.Bits) < 1 && len(chain.CreateAuxBlock.ResponseKeys.Target) < 1 {
			return errors.New("Chains[" + strconv.Itoa(index) + "].CreateAuxBlock.ResponseKeys.Bits and chain.CreateAuxBlock.ResponseKeys.Target cannot be empty together")
		}

		if chain.ChainID == 0 && len(chain.CreateAuxBlock.ResponseKeys.ChainID) < 1 {
			return errors.New("Chains[" + strconv.Itoa(index) + "].ChainID and Chains[" + strconv.Itoa(index) + "].CreateAuxBlock.ResponseKeys.ChainID all missing")
		}

		if chain.ChainID != 0 && len(chain.CreateAuxBlock.ResponseKeys.ChainID) >= 1 {
			glog.Info("Chains[" + strconv.Itoa(index) + "].ChainID and Chains[" + strconv.Itoa(index) + "].CreateAuxBlock.ResponseKeys.ChainID all defined, use Chains[" + strconv.Itoa(index) + "].CreateAuxBlock.ResponseKeys.ChainID first")
		}
	}

	return nil
}

// LoadFromFile Load configuration from file
func (conf *ConfigData) LoadFromFile(file string) (err error) {

	configJSON, err := ioutil.ReadFile(file)

	if err != nil {
		return
	}

	err = json.Unmarshal(configJSON, conf)
	if err != nil {
		return
	}

	err = conf.Check()
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
