package main

import (
	"flag"
	"net/http"
	_ "net/http/pprof"

	"github.com/golang/glog"
)

func main() {
	// Parse command line arguments
	configFilePath := flag.String("config", "./config.json", "Path of config file")
	// Running state file saved during non-stop upgrade
	runtimeFilePath := flag.String("runtime", "", "Path of runtime file, use for zero downtime upgrade.")
	flag.Parse()

	// read configuration file
	var configData ConfigData
	err := configData.LoadFromFile(*configFilePath)

	if err != nil {
		glog.Fatal("load config failed: ", err)
		return
	}

	// Read runtime state
	var runtimeData RuntimeData

	if len(*runtimeFilePath) > 0 {
		runtimeData.LoadFromFile(*runtimeFilePath)
	}

	// Enable HTTP Debug
	if configData.EnableHTTPDebug {
		go func() {
			glog.Info("HTTP debug enabled: ", configData.HTTPDebugListenAddr)
			http.ListenAndServe(configData.HTTPDebugListenAddr, nil)
		}()
	}

	sessionManager, err := NewStratumSessionManager(configData, runtimeData)
	if err != nil {
		glog.Fatal("create session manager failed: ", err)
		return
	}
	sessionManager.Run(runtimeData)
}
