# Init User Coin

Incremental initialization of user currency records in zookeeper by pulling the sub-account name/puid list of each currency.

This procedure is optional and depends on the user system architecture of the mining pool. If your sub-account list does not distinguish between currencies at all, you don't need to deploy this program, and you can directly use the scheduled task of `Switcher API Server` to initialize the currency record.

### Sub-account name/puid list interface

The `sub-account name/puid list` required by this program is the same as that required by [sserver](https://github.com/btccom/btcpool/blob/master/src/sserver/sserver.cfg) in BTCPool. as follows:

#### Interface conventions

Suppose the configuration file is
```json
{
    "UserListAPI": {
        "btc": "http://127.0.0.1:8000/btc-userlist.php",
        "bcc": "http://127.0.0.1:8000/bcc-userlist.php"
    },
    "IntervalSeconds": 10,
    ...
}
```

Then the program will start two `goroutine` (threads), and access the sub-account name/puid list interface of `btc` and `bcc` at the same time.

Taking the sub-account name/puid list interface of `btc` as an example, the actual URL of the first visit is:
```
http://127.0.0.1:8000/btc-userlist.php?last_id=0
```
The interface returns a complete list of users/puids, such as:
```
{
    "err_no": 0,
    "err_msg": null,
    "data": {
        "aaa": 1,
        "bbb": 2,
        "mmm_btc": 4,
        "vvv": 5,
        "ddd": 6
    }
}
```
The program will traverse the list and set the mined currency of users `aaa`, `bbb`, `vvv`, `ddd` to `btc`. This program is only responsible for initialization, not for subsequent currency switching, so it simply thinks that the currency mined by the user appearing in the `btc` list is `btc`, and the currency mined by the user appearing in the `bcc` list It is `bcc`.

Also, subaccount names with underscores will be skipped, so the program will not set the mined currency of the `mmm_btc` subaccount.

After waiting `IntervalSeconds` seconds, the program will visit the following URL again:
```
http://127.0.0.1:8000/btc-userlist.php?last_id=6
```
Where `6` is the largest puid obtained last time.

If no user with `puid` greater than `6` is registered, the interface returns an empty `data` object:
```json
{
    "err_no": 0,
    "err_msg": null,
    "data": {
    }
}
```
Otherwise, the interface returns users whose `puid` is greater than `6`, such as:
```json
{
    "err_no": 0,
    "err_msg": null,
    "data": {
        "xxx": 7
    }
}
```
After that, the mined currency of user `xxx` will be set to `btc`, and `last_id` will be changed to `7`.

##### Remark

1. It is safe to restart the program. Although the program will start traversing the list of subaccounts again, the program will not write records for subaccounts that already exist in `zookeeper`. Therefore, the restart of the program will not affect the user's subsequent currency switching.

2. The program can run all the time, so that it can incrementally initialize the currency of the newly registered user.

3. If the same subaccount appears in both `btc` and `bcc` lists, the program initializes it to `btc` or `bcc` depending on which side of the record it processes first. If all your sub-accounts will appear on both sides at the same time, and the puid is the same, or your sub-account list does not distinguish currencies at all, you do not need to deploy this program, just use [Switcher API Server](../switcherAPIServer#Timer The timed task of the task) can initialize the currency record.

##### About sub-account names with underscores

Subaccount names with underscores can be used in cases where "the user actually has a subaccount under the `btc` and `bcc` currencies, but wants to make the user feel like they have only one subaccount". The specific approach is:
1. Suppose the user already has a sub-account under the `btc` currency, which is `mmm`.
2. The user operates the currency switching function and wants to switch to `bcc`. At this point, the system automatically creates a sub-account under the `bcc` currency for the user named `mmm_bcc`. The subaccount may have a different `puid` than the `mmm` subaccount.
3. The system calls the [currency switch API](../switcherAPIServer#single user switch) at the same time to switch the currency of user `mmm` to `bcc`, such as `http://10.0.0.12:8082/switch? puname=mmm&coin=bcc`.
4. At the same time, make sure that the currency of the user `mmm` returned by [UserCoinMapURL](../switcherAPIServer#interface convention) is also `bcc`. In addition, subaccount names with underscores should not appear in the returned result of `UserCoinMapURL` (because logically underlined and non-underlined subaccounts are the same subaccount).
5. The user still uses the sub-account name `mmm` to connect to the mining pool. At this point, `stratumSwitcher` will forward the connection to `bcc`'s `sserver`. But there is no sub-account named `mmm` at `bcc`, so the miner authentication will fail. At this point, `stratumSwitcher` will automatically convert the sub-account name to `mmm_bcc` and try again, and it will succeed. The user's existing miners will also be switched to the `mmm_bcc` sub-account of the `bcc` currency.

#### reference implementation

Here is an example implementing `UserListAPI`：https://github.com/btccom/btcpool/issues/16#issuecomment-278245381

### build & run

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
GIT_TERMINAL_PROMPT=1 go get github.com/BobZombiE69/btcpool-go-modules/initUserCoin
```

Edit configuration file

```bash
mkdir /work/golang/initUserCoin
mkdir /work/golang/initUserCoin/log
cp /work/golang/src/github.com/BobZombiE69/btcpool-go-modules/initUserCoin/config.default.json /work/golang/initUserCoin/config.json
vim /work/golang/initUserCoin/config.json
```

Create supervisor entry

```bash
vim /etc/supervisor/conf.d/switcher-inituser.conf
```

```conf
[program:switcher-inituser]
directory=/work/golang/initUserCoin
command=/work/golang/bin/initUserCoin -config=/work/golang/initUserCoin/config.json -log_dir=/work/golang/initUserCoin/log -v 2
autostart=true
autorestart=true
startsecs=6
startretries=20

redirect_stderr=true
stdout_logfile_backups=5
stdout_logfile=/work/golang/initUserCoin/log/stdout.log
```

run

```bash
supervisorctl reread
supervisorctl update
supervisorctl status
```

#### renew

```bash
export GOPATH=/work/golang
GIT_TERMINAL_PROMPT=1 go get -u github.com/BobZombiE69/btcpool-go-modules/initUserCoin
diff /work/golang/src/github.com/BobZombiE69/btcpool-go-modules/initUserCoin/config.default.json /work/golang/initUserCoin/config.json
```
