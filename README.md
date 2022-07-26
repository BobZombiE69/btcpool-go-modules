# [User Chain API Server] (Userchainapiserver/)

Merge from two modules:
* [Switcher API Server] (UserchainaPiserver/Switcherapiserver/)
  API that provides stratum switching
* [Init User Coin]
  User currency record in Zookeeper

# [MERGED MINING ProXY]

Multi -currency combined mining agents support domain name coins, Elastos, etc. with Bitcoin and mining at the same time.

# [Init Nicehash] (Initnicehash/)

Initialize the Nicehash configuration in Zookeeper, to obtain the minimum difficulty required by each algorithm by calling the Nicehash API, and write it to the use of SSERVER.

# [Chain Switcher] (Chainswitcher/)
Send a currency to a currency automatic switching command to SSERVER.

# [Stratum Switcher] (Stratumswitcher/)

The Stratum proxy of the currency can be switched to work with BTCPOOL.

** has been abandoned. SSERVER in the BTCPOOL project now has a currency switching function and no longer needs Stratum Switcher. **

* [BTCPool for Bitcoin Cash] (https://github.com/btccom/bccpool)
* [Btcpool for bitcoin] (https://github.com/btccom/btcpool)