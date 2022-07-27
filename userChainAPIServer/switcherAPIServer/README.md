# Switcher API Server

This process is used to modify the currency record in zookeeper in order to control the currency switching of StratumSwitcher. This process has two working modes, one is to pull the latest user currency information through scheduled tasks, and the other is to actively push user currency information by calling the API provided by the process.

## Scheduled tasks

Set `EnableCronJob` to `true` in the configuration file to start the scheduled task. After that, the process will pull `UserCoinMapURL` every `CronIntervalSeconds` to obtain the latest user currency information.

### Interface convention

Assuming `UserCoinMapURL` is `http://127.0.0.1:8000/usercoin.php`, the actual URL accessed by the program for the first time is:
```
http://127.0.0.1:8000/usercoin.php?last_date=0
```

The interface returns a JSON string containing all users and the currency they are mining (whether they have been switched or not), as follows:
```json
{
    "err_no": 0,
    "data": {
        "user_coin": {
            "user1": "btc",
            "user2": "bcc",
            "user3": "bcc",
            "user4": "btc"
        },
        "now_date": 1513239055
    }
}
```
Among them, `user1`, `user2`, `user3` are the sub-account names, `btc` and `bcc` are the currency, and `now_date` is the current system time of the server.

After `CronIntervalSeconds` seconds set in the configuration file, the program will visit the following URL again:
```
http://127.0.0.1:8000/usercoin.php?last_date=1513239055
```
Among them, `1513239055` is the `now_date` returned by the server last time.

At this point, the server can judge according to the `last_date` provided by the program. If no user has switched between the `last_date` and the present, it will return an empty `user_coin` object:
```json
{
    "err_no": 0,
    "data": {
        "user_coin": {},
        "now_date": 1513239064
    }
}
```
> Note: The `user_coin` array cannot be returned, such as `"user_coin":[]`, otherwise the program will generate a warning in the log. When implementing the interface with a PHP array, please cast the type of the `user_coin` member to an object before outputting.

Otherwise, return the user who switched during this period and the currency after switching:
```json
{
    "err_no": 0,
    "data": {
        "user_coin": {
            "user1": "bcc",
            "user3": "btc"
        },
        "now_date": 1513239064
    }
}
```

Note: If performance is not affected, the server can also ignore the `last_date` parameter and always return all users and the coins they are mining, regardless of whether or when they have switched.

### reference implementation

The reference implementation of `UserCoinMapURL` is as follows:

```php
<?php
header('Content-Type: application/json');

$last_id = (int) $_GET['last_id'];

$coins = ["btc", "bcc"];

$users = [
    'hu60' => $coins[rand(0,1)],
    'YihaoTest' => $coins[rand(0,1)],
    'YihaoTest3' => $coins[rand(0,1)],
    'testpool' => $coins[rand(0,1)],
];

if ($last_id >= count($users)) {
    $users = [];
}

echo json_encode(
    [
        'err_no' => 0,
        'err_msg' => null,
        'data' => (object) $users,
    ]
);
```

## API Documentation

Set EnableAPIServer to true in the configuration file to enable the API service. External users can call this API to actively push switching messages when users initiate switching requests, so that StratumSwitcher can switch currencies at the first time.

There are currently two calling methods:

### Single User Switching

#### verification method
HTTP Basic Authentication

#### request URL
http://hostname:port/switch

#### request method
GET or POST

#### parameters
| Name | Type | Meaning |
| ------| -----| --------|
| puname | string | Sub account name |
| coin | string | Currency |
#### example

Switch the sub-account aaaa to btc:
```bash
curl -u admin:admin 'http://127.0.0.1:8082/switch?puname=aaaa&coin=btc'
```

Switch the subaccount aaaa to bcc:
```bash
curl -u admin:admin 'http://10.0.0.12:8082/switch?puname=aaaa&coin=bcc'
```

The returned result of this API:

success:
```json
{"err_no":0, "err_msg":"", "success":true}
```

fail:
```
{"err_no": non-zero integer, "err_msg": "error message", "success":false}
```
E.g
```json
{"err_no":104,"err_msg":"coin is inexistent","success":false}
```

### batch switch

#### verification method
HTTP Basic Authentication

#### request URL
* http://hostname:port/switch/multi-user
* http://hostname:port/switch-multi-user

#### request method
POST

`Content-Type: application/json`

#### Request Body Content

```json
{
    "usercoins": [
        {
            "coin": "Coin 1",
            "punames": [
                "User1",
                "User2",
                "User3",
                ...
            ]
        },
        {
            "coin": "Coin 2",
            "punames": [
                "User4",
                "User5",
                ...
            ]
        },
        ...
    ]
}
```

#### example

Sub-accounts a, b, c switch to btc, d, e switch to bcc:
```bash
curl -u admin:admin -d '{"usercoins":[{"coin":"btc","punames":["a","b","c"]},{"coin":"bcc","punames":["d","e"]}]}' 'http://127.0.0.1:8082/switch/multi-user'
```

The returned result of this API:

All sub-accounts have been switched successfully:
```json
{"err_no":0, "err_msg":"", "success":true}
```

Any sub-account switching fails:
```
{"err_no": non-zero integer, "err_msg": "error message", "success": false}
```
E.g
```json
{"err_no":108,"err_msg":"usercoins is empty","success":false}
```

### Get sub-pool Coinbase information and block address

#### verification method
HTTP Basic Authentication

#### request URL
*http://hostname:port/subpool/get-coinbase
*http://hostname:port/subpool-get-coinbase

#### request method
POST

`Content-Type: application/json`

#### Request Body Content
```json
{
	"coin": "currency",
	"subpool_name": "Subpool name"
}
```

#### response

success：
```json
{
	"success": true,
	"err_no": 0,
	"err_msg": "success",
	"subpool_name": "Subpool name",
	"old": {
		"coinbase_info": "coinbase information",
		"payout_addr": "Explosive block address"
	}
}
```

fail:
```json
{
	"err_no": error code,
	"err_msg": "error message",
	"success": false
}
```

Data race (may occur when multiple get-coinbase/update-coinbase are called at the same time, stagger the time and try again to succeed):
```
{
	"err_no": 500,
	"err_msg": "data has been updated at query time",
	"success": false
}
```

example:
```json
curl -uadmin:admin -d'{"coin":"btc","subpool_name":"pool3"}' http://localhost:8080/subpool/get-coinbase
{
	"success": true,
	"err_no": 0,
	"err_msg": "success",
	"subpool_name": "pool3",
	"old": {
		"coinbase_info": "tigerxx",
		"payout_addr": "34woZDygXWqaVPnNxp5SUnbN6RNQ5koBt4"
	}
}

curl -uadmin:admin -d'{"coin":"bch","subpool_name":"pool3"}' http://localhost:8080/subpool/get-coinbase
{
	"err_no": 404,
	"err_msg": "subpool 'pool3' does not exist",
	"success": false
}

curl -uadmin:admin -d'{"coin":"bch","subpool_name":"pool3"}' http://localhost:8080/subpool/get-coinbase
{
	"err_no": 500,
	"err_msg": "data has been updated at query time",
	"success": false
}
```

### Update sub-pool Coinbase information and block address

#### verification method
HTTP Basic Authentication

#### request URL
* http://hostname:port/subpool/update-coinbase
* http://hostname:port/subpool-update-coinbase

#### request method
POST

`Content-Type: application/json`

#### Request Body Content
```json
{
	"coin": "currency",
	"subpool_name": "Subpool name",
	"payout_addr": "Explosive block address",
	"coinbase_info": "coinbase information"
}
```

#### response

success:
```json
{
	"success": true,
	"err_no": 0,
	"err_msg": "success",
	"subpool_name": "Subpool name",
	"old": {
		"coinbase_info": "old coinbase info",
		"payout_addr": "old block address"
	},
	"new": {
		"coinbase_info": "New coinbase information",
		"payout_addr": "New block address"
	}
}
```

fail:
```json
{
	"success": false,
	"err_no": error code,
	"err_msg": "error message",
	"subpool_name": "Subpool name",
	"old": {
		"coinbase_info": "old coinbase info",
		"payout_addr": "old block address"
	},
	"new": {
		"coinbase_info": "",
		"payout_addr": ""
	}
}
```

Request parameter error:
```json
{
	"err_no": error code,
	"err_msg": "error message",
	"success": false
}
```

example:
```json
curl -uadmin:admin -d'{"coin":"btc","subpool_name":"pool3","payout_addr":"bc1qjl8uwezzlech723lpnyuza0h2cdkvxvh54v3dn","coinbase_info":"tiger"}' http://localhost:8080/subpool/update-coinbase
{
	"success": true,
	"err_no": 0,
	"err_msg": "success",
	"subpool_name": "pool3",
	"old": {
		"coinbase_info": "hellobtc",
		"payout_addr": "34woZDygXWqaVPnNxp5SUnbN6RNQ5koBt4"
	},
	"new": {
		"coinbase_info": "tiger",
		"payout_addr": "bc1qjl8uwezzlech723lpnyuza0h2cdkvxvh54v3dn"
	}
}

curl -uadmin:admin -d'{"coin":"btc","subpool_name":"pool3","payout_addr":"bc0qjl8uwezzlech723lpnyuza0h2cdkvxvh54v3dn","coinbase_info":"tiger"}' http://localhost:8080/subpool/update-coinbase
{
	"success": false,
	"err_no": 500,
	"err_msg": "invalid payout address",
	"subpool_name": "pool3",
	"old": {
		"coinbase_info": "tiger",
		"payout_addr": "bc1qjl8uwezzlech723lpnyuza0h2cdkvxvh54v3dn"
	},
	"new": {
		"coinbase_info": "",
		"payout_addr": ""
	}
}

curl -uadmin:admin -d'{"coin":"btc","subpool_name":"pool4","payout_addr":"bc1qjl8uwezzlech723lpnyuza0h2cdkvxvh54v3dn","coinbase_info":"tiger"}' http://localhost:8080/subpool/update-coinbase
{
	"err_no": 404,
	"err_msg": "subpool 'pool4' does not exist",
	"success": false
}
```


## build & run

install golang

```bash
mkdir ~/source
cd ~/source
wget http://storage.googleapis.com/golang/go1.10.3.linux-amd64.tar.gz
cd /usr/local
tar zxf ~/source/go1.10.3.linux-amd64.tar.gz
ln -s /usr/local/go/bin/go /usr/local/bin/go
```

Construct

```bash
mkdir -p /work/golang
export GOPATH=/work/golang
GIT_TERMINAL_PROMPT=1 go get github.com/BobZombiE69/btcpool-go-modules/switcherAPIServer
```

Edit configuration file

```bash
mkdir /work/golang/switcherAPIServer
mkdir /work/golang/switcherAPIServer/log
cp /work/golang/src/github.com/BobZombiE69/btcpool-go-modules/switcherAPIServer/config.default.json /work/golang/switcherAPIServer/config.json
vim /work/golang/switcherAPIServer/config.json
```

Create supervisor entry

```bash
vim /etc/supervisor/conf.d/switcher-api.conf
```

```conf
[program:switcher-api]
directory=/work/golang/switcherAPIServer
command=/work/golang/bin/switcherAPIServer -config=/work/golang/switcherAPIServer/config.json -log_dir=/work/golang/switcherAPIServer/log -v 2
autostart=true
autorestart=true
startsecs=6
startretries=20

redirect_stderr=true
stdout_logfile_backups=5
stdout_logfile=/work/golang/switcherAPIServer/log/stdout.log
```

run

```bash
supervisorctl reread
supervisorctl update
supervisorctl status
```

## renew

```bash
export GOPATH=/work/golang
GIT_TERMINAL_PROMPT=1 go get -u github.com/BobZombiE69/btcpool-go-modules/switcherAPIServer
diff /work/golang/src/github.com/BobZombiE69/btcpool-go-modules/switcherAPIServer/config.default.json /work/golang/switcherAPIServer/config.json
```
