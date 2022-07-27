# [User Chain API Server](userChainAPIServer/)

Merge from two modules:
* [Switcher API Server](userChainAPIServer/switcherAPIServer/)
  Provides an API to trigger Stratum switching
* [Init User Coin](userChainAPIServer/initUserCoin/)
  Initialize the user currency record in zookeeper

# [Merged Mining Proxy](mergedMiningProxy/)

Multi-currency joint mining agent, support Namecoin, Elastos, etc., and Bitcoin joint mining at the same time.

# [Init NiceHash](initNiceHash/)

Initialize the NiceHash configuration in ZooKeeper, obtain the minimum diffiStandby sserver to use. by each algorithm by calling the NiceHash API, and write to ZooKeeper to备 sserver 来使用。

# [Chain Switcher](chainSwitcher/)
Send currency automatic switching command to sserver.

# [Stratum Switcher](stratumSwitcher/)

A currency-switchable Stratum agent for working with BTCPool.

**Obsolete, the sserver in the btcpool project now has the function of currency switching directly, no need for Stratum Switcher anymore. **

* [BTCPool for Bitcoin Cash](https://github.com/btccom/bccpool)
* [BTCPool for Bitcoin](https://github.com/btccom/btcpool)
