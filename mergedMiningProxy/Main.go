package main

import (
	"flag"

	"github.com/golang/glog"
)

func main() {
	// parse command args
	configFilePath := flag.String("config", "./config.json", "Path of config file")
	flag.Parse()

	// read configuration file
	var configData ConfigData
	err := configData.LoadFromFile(*configFilePath)
	if err != nil {
		glog.Fatal("load config failed: ", err)
		return
	}

	// Run the task generator
	auxJobMaker := NewAuxJobMaker(configData.AuxJobMaker, configData.Chains)
	auxJobMaker.Run()
	// Start RPC Server
	runHTTPServer(configData.RPCServer, auxJobMaker)
}
