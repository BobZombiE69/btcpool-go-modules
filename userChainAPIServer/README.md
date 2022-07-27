# User Chain API Server
Combining two modules, please see the introduction of specific submodules:
*[Switcher API Server](switcherAPIServer/)
  Provides an API to trigger Stratum switching
*[Init User Coin](initUserCoin/)
  Initialize the user currency record in zookeeper
## Construct
```
go get -u github.com/BobZombiE69/btcpool-go-modules/userChainAPIServer
```

## run
```
cp config.default.json config.json
$GOPATH/bin/userChainAPIServer --config config.json --logtostderr -v 2
```

# Docker

## Construct
```
cd btcpool-go-modules/userChainAPIServer
docker build -t btcpool-user-chain-api-server -f Dockerfile ..
```

## run
```
docker run -it --rm --network=host \
  -e AvailableCoins='ubtc,btc,bcc,auto' \
  -e UserListAPI_ubtc='http://localhost:8000/userlist-ubtc.php' \
  -e UserListAPI_btc='http://localhost:8000/userlist-btc.php' \
  -e UserListAPI_bcc='http://localhost:8000/userlist-bch.php' \
  -e ZKBroker='10.0.1.176:2181,10.0.1.175:2181,10.0.1.174:2181' \
  -e ZKSwitcherWatchDir='/stratumSwitcher/btcbcc/' \
  -e EnableAPIServer='true' \
  -e APIUser='switchapi' \
  -e APIPassword='admin' \
  -e ListenAddr='0.0.0.0:8082' \
  -e EnableCronJob='true' \
  -e UserCoinMapURL='http://localhost:8000/usercoin.php' \
  -e StratumServerCaseInsensitive='true' \
  btcpool-user-chain-api-server:latest -logtostderr -v 2

# daemon
docker run -it --name user-chain-api-server --network=host --restart always -d \
  -e AvailableCoins='ubtc,btc,bcc,auto' \
  -e UserListAPI_ubtc='http://localhost:8000/userlist-ubtc.php' \
  -e UserListAPI_btc='http://localhost:8000/userlist-btc.php' \
  -e UserListAPI_bcc='http://localhost:8000/userlist-bch.php' \
  -e ZKBroker='10.0.1.176:2181,10.0.1.175:2181,10.0.1.174:2181' \
  -e ZKSwitcherWatchDir='/stratumSwitcher/btcbcc/' \
  -e EnableAPIServer='true' \
  -e APIUser='switchapi' \
  -e APIPassword='admin' \
  -e ListenAddr='0.0.0.0:8082' \
  -e EnableCronJob='true' \
  -e UserCoinMapURL='http://localhost:8000/usercoin.php' \
  -e StratumServerCaseInsensitive='true' \
  btcpool-user-chain-api-server:latest -logtostderr -v 2
```

The currency `auto` is optional, used for machine gun switching, and does not need to be configured in the `chains` of `sserver`. `sserver` only needs to turn on the machine gun switch function (`auto_switch_chain`) to recognize the currency `auto`.

If you need the automatic registration function, you can use the following configuration:
```
docker run -it --name user-chain-api-server --network=host --restart always -d \
  -e AvailableCoins='ubtc,btc,bcc,auto' \
  -e UserListAPI_ubtc='http://localhost:8000/userlist-ubtc.php' \
  -e UserListAPI_btc='http://localhost:8000/userlist-autoreg.php' \
  -e UserListAPI_bcc='http://localhost:8000/userlist-bch.php' \
  -e ZKBroker='10.0.1.176:2181,10.0.1.175:2181,10.0.1.174:2181' \
  -e ZKSwitcherWatchDir='/stratumSwitcher/btcbcc/' \
  -e EnableAPIServer='true' \
  -e APIUser='switchapi' \
  -e APIPassword='admin' \
  -e ListenAddr='0.0.0.0:8082' \
  -e EnableCronJob='true' \
  -e UserCoinMapURL='http://localhost:8000/usercoin.php' \
  -e StratumServerCaseInsensitive='true' \
  -e EnableUserAutoReg="true" \
  -e ZKAutoRegWatchDir="/stratumSwitcher/btcbcc_autoreg/" \
  -e UserAutoRegAPI_IntervalSeconds=10 \
  -e UserAutoRegAPI_URL="http://localhost:8000/autoreg.php" \
  -e UserAutoRegAPI_User="" \
  -e UserAutoRegAPI_Password="" \
  -e UserAutoRegAPI_DefaultCoin="btc" \
  -e UserAutoRegAPI_PostData='{"sub_name": "{sub_name}", "region_name": "all", "currency": "btc"}' \
  btcpool-user-chain-api-server:latest -logtostderr -v 2
```
