package main

import (
	"flag"

	initusercoin "github.com/BobZombiE69/btcpool-go-modules/userChainAPIServer/initUserCoin"
	switcherapiserver "github.com/BobZombiE69/btcpool-go-modules/userChainAPIServer/switcherAPIServer"
)

func main() {
	// Parse command line arguments
	configFilePath := flag.String("config", "./config.json", "Path of config file")
	flag.Parse()

	go switcherapiserver.Main(*configFilePath)
	initusercoin.Main(*configFilePath)
}
