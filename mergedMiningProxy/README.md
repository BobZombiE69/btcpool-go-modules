# Merged Mining Proxy

A bitcoin merge-mining agent for simultaneously mining multiple coins that comply with the [Bitcoin Merged Mining Standard](https://en.bitcoin.it/wiki/Merged_mining_specification).

It can be used to mine Namecoin, Elastos, etc. at the same time in the same Bitcoin mining pool.

### build & run

#### install golang

```bash
mkdir ~/source
cd ~/source
wget http://storage.googleapis.com/golang/go1.10.3.linux-amd64.tar.gz
cd /usr/local
tar zxf ~/source/go1.10.3.linux-amd64.tar.gz
ln -s /usr/local/go/bin/go /usr/local/bin/go
```

#### Construct

```bash
mkdir -p /work/golang
export GOPATH=/work/golang
GIT_TERMINAL_PROMPT=1 go get github.com/BobZombiE69/btcpool-go-modules/mergedMiningProxy
```

#### Edit configuration file

```bash
mkdir /work/golang/mergedMiningProxy
mkdir /work/golang/mergedMiningProxy/log
cp /work/golang/src/github.com/BobZombiE69/btcpool-go-modules/mergedMiningProxy/config.default.json /work/golang/mergedMiningProxy/config.json
vim /work/golang/mergedMiningProxy/config.json
```

##### Configuration file details:

Note: JSON files do not support comments. If you want to copy the following configuration files, please **delete all comments**first.
```js
{
    "RPCServer": {
        "ListenAddr": "0.0.0.0:8999", // listen ip and port
        "User": "admin",  // BasicAuthenticationUsername
        "Passwd": "admin", // Basic authentication password
        "MainChain":"BTC",  // Specify the main chain type of combined mining, such as：bitcoin => "BTC", litecoin => "LTC"
        "PoolDb": {
            "host" : "127.0.0.1",
            "port" : 3306,
            "username" : "root",
            "password" : "root",
            "dbname" : "bpool_local_db"
        }
    },
    "AuxJobMaker": {
        "CreateAuxBlockIntervalSeconds": 5, // Frequency of updating merged mining tasks (seconds)
        "AuxPowJobListSize": 1000, // The number of reserved combined mining tasks (assuming that the client calls the getauxblock interface of this program every 5 seconds, 1000 tasks are 5000 seconds)
//Optional, the maximum Target (ie minimum difficulty) allowed for the task. If the task Target is greater than this value (difficulty is less than the difficulty corresponding to this value), it is replaced with this value.
        //It is used to control the block generation speed of the chain with very low difficulty. For example, if it is set to "00000000ffffffffffffffffffffffffffffffffffffffffffffffffffffff", the system cannot handle the explosion-proof block speed too fast.
        //Note: The unreasonable setting of this value will cause the block to be exploded normally. If this feature is not required, keep the default value or delete this option.
        "MaxJobTarget": "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff" 
    },
    "Chains": [
        // Any number of chains can be added
        {
            "Name": "Namecoin", // Chain name, only used for logging, the content can be customized
            "AuxTableName" :"found_nmc_blocks",// Table name for storing auxpow related information
            "RPCServer": {
                "URL": "http://127.0.0.1:8444/", // Merged Mining RPC Server
                "User": "test", // BasicAuthenticationUsername
                "Passwd": "123" // Basic authentication password
            },
            // Define the RPC that creates the merged mining task
            // Because different blockchains may have different RPCs (including methods, parameters, and return values), they are defined through configuration files
            "CreateAuxBlock": {
                "Method": "getauxblock", // method name
                "Params": [], // Parameters, which can be of any type (array, object, string, etc.)
                // Return value key name map
                // The return value of the RPC must be similar to the following structure, where the key name can be different from the example below.
                // Not all keys are required, currently only "hash" and "bits" are required (key names can be different).
                // "chainid" is required in some cases (see description below).
                /*
                    {
                        "result": {
                            "hash": "47478e2d769c26e702108b624dd403bfcae669cd51171aed7a85b985805ab032",
                            "chainid": 1,
                            "previousblockhash": "05f9d32813005597ae98c9c57427ff708be9651ae81e899caafacc36d5520f39",
                            "coinbasevalue": 5000000000,
                            "bits": "207fffff",
                            "height": 41,
                            "_target": "0000000000000000000000000000000000000000000000000000000000ffff7f"
                        },
                        "error": null,
                        "id": "curltest"
                    }
                */
                // Define the key name of each data in the return value
                // If there is no optional data in the returned value, delete the corresponding key-value directly from the {} below
                // If there is no required data in the return value, the blockchain node is not compatible with the current version of the program
                "ResponseKeys": {
                    "Hash": "hash", // The block header hash for merged mining, required.
                    "ChainID": "chainid", // The chain id, if the specific value of the chain id is not defined in the configuration file, the key name must exist.
                    "Bits": "bits", // The difficulty required for combined mining is required. Adopt the encoding method of [nBits field in the Bitcoin block header](https://bitcoin.org/en/developer-reference#target-nbits)
                    "Height": "height", // Block height, optional. Currently only used for logging.
                    "PrevBlockHash": "previousblockhash", //The parent block hash of the current block, optional. Currently only used for logging.
                    "CoinbaseValue": "coinbasevalue" // The reward for mining this block, optional. Currently only used for logging.
                }
            },
            // Define the RPC for submitting merged mining results
            "SubmitAuxBlock": {
                "Method": "getauxblock", // 方法名
//parameters
                //Two types of parameters are supported:
                //1. JSON-RPC 1.0 array parameters
                //1. JSON-RPC 2.0 named parameters (object, map, key-value pair)
                //If the value of a parameter is a string, it can contain "variable" tags, which will be replaced with the corresponding value when submitting.
                //There are currently only two "variable" tags available:
                //{hash-hex} combined mining block header hash (obtained from CreateAuxBlock to indicate which block was mined)
                //{aux-pow-hex} The hex representation of the proof-of-work data. This data structure follows the Bitcoin Merged Mining Standard: https://en.bitcoin.it/wiki/Merged_mining_specification#Aux_proof-of-work_block
//Arguments can contain text (constants) other than "variable" tags, or arguments of non-string type. Numeric values, nulls, arrays, objects, etc. are all allowed.
                //But note that only "variable" tokens in strings in the outermost array/object will be replaced.
                //If a blockchain node requires a block header hash or proof-of-work to be placed into a deep array or object, it is not compatible with the current version of the program.                "Params": [
                    "{hash-hex}",
                    "{aux-pow-hex}"
                ]
            }
        },
        {
            "Name": "Namecoin ChainID 7",
//The chain id of the blockchain can be forcibly modified in this program
            //This option is usually only used for debugging, or can be used for blockchain nodes that are compatible with RPC return values ​​that do not contain chain id
            //If the chain id defined here is different from the actual requirements of the blockchain, the result of the combined mining will be rejected by the blockchain node overloaded chain id
            "ChainID": 7, // overloaded chain id
            "RPCServer":{
                "URL": "http://127.0.0.1:9444/",
                "User": "test",
                "Passwd": "123"
            },
            "CreateAuxBlock": {
                "Method": "createauxblock",
                "Params": [ "my2dxGb5jz43ktwGxg2doUaEb9WhZ9PQ7K" ],
                // The "ChainID" field does not (and cannot) appear here, otherwise the overloaded chain id above will not take effect
                "ResponseKeys": {
                    "Hash": "hash",
                    "Bits": "bits",
                    "Height": "height",
                    "PrevBlockHash": "previousblockhash",
                    "CoinbaseValue": "coinbasevalue"
                }
            },
            "SubmitAuxBlock": {
                "Method": "submitauxblock",
                "Params": [
                    "{hash-hex}",
                    "{aux-pow-hex}"
                ]
            }
        },
        {
            "Name": "Elastos Regtest",
            "AuxTableName" :"found_Elastos_blocks",
            "RPCServer":{
                "URL": "http://127.0.0.1:4336/",
                "User": "test",
                "Passwd": "123"
            },
            "CreateAuxBlock": {
                "Method": "createauxblock",
                // Named parameters are used here
                "Params": {
                    "paytoaddress": "8VYXVxKKSAxkmRrfmGpQR2Kc66XhG6m3ta"
                },
                "ResponseKeys": {
                    "Hash": "hash",
                    "ChainID": "chainid",
                    "Bits": "bits",
                    "Height": "height",
                    "PrevBlockHash": "previousblockhash",
                    "CoinbaseValue": "coinbasevalue"
                }
            },
            "SubmitAuxBlock": {
                "Method": "submitauxblock",
//named parameters
                //Note that only the "variable" tag in the value of the first level object can be replaced
                "Params": {
                    "blockhash": "{hash-hex}",
                    "auxpow": "{aux-pow-hex}"
                }
            }
        }
    ]
}
```

#### 创建supervisor条目

```bash
vim /etc/supervisor/conf.d/merged-mining-proxy.conf
```

```conf
[program:merged-mining-proxy]
directory=/work/golang/mergedMiningProxy
command=/work/golang/bin/mergedMiningProxy -config=/work/golang/mergedMiningProxy/config.json -log_dir=/work/golang/mergedMiningProxy/log -v 2
autostart=true
autorestart=true
startsecs=6
startretries=20

redirect_stderr=true
stdout_logfile_backups=5
stdout_logfile=/work/golang/mergedMiningProxy/log/stdout.log
```

#### 运行

```bash
supervisorctl reread
supervisorctl update
supervisorctl status
```

#### 更新

```bash
export GOPATH=/work/golang
GIT_TERMINAL_PROMPT=1 go get -u github.com/BobZombiE69/btcpool-go-modules/mergedMiningProxy
diff /work/golang/src/github.com/BobZombiE69/btcpool-go-modules/mergedMiningProxy/config.default.json /work/golang/mergedMiningProxy/config.json
```

### Call the proxy's RPC

Support `getauxblock`, `createauxblock`, `submitauxblock` and other methods. The parameter list of the method is the same as that of Namecoin.

If not the above method, the method will be forwarded to the first blockchain node defined in the configuration file and return the result as-is.

A special case is the `help` method. In order to be compatible with the BTCPool `nmcauxmaker`'s Namecoin node version checking logic, the help method appends a description of the `createauxblock` and `submitauxblock` methods to the original return value.

like:
```bash
# get task
curl -v --user admin:admin --data-binary '{"id":null,"method":"getauxblock","params":[]}' -H 'content-type: application/json' http://localhost:8999/

# Submit a task
curl -v --user admin:admin --data-binary '{"id":1,"method":"getauxblock","params":["6c8adaefa07ab5ff14ddff1b8e2bae4ecfc59ef0a985064bd202565106ff054b","02000000010000000000000000000000000000000000000000000000000000000000000000ffffffff4b039ccd09041b96485b742f4254432e434f4d2ffabe6d6d6c8adaefa07ab5ff14ddff1b8e2bae4ecfc59ef0a985064bd202565106ff054b020000004204cb9a010388711000000000000000ffffffff0200000000000000001976a914c0174e89bd93eacd1d5a1af4ba1802d412afc08688ac0000000000000000266a24aa21a9ede2f61c3f71d1defd3fa999dfa36953755c690689799962b48bebd836974e8cf9000000002d6009ef30ae316bcebe42ea7f4feaf995fb34211aa80b9835e06b0388769ce6000000000000000000000000002075cc0a4e259833d348dd282c00a61ab112bea0e02d1ac85e4773d08a01b87b3f2dc921f1fd927d473649cbb7115debb95de77455401566d56b12a94cbfca8dff1b96485bffff7f201b96485b"]}' -H 'content-type: application/json' http://localhost:8999/

# Get the task (the wallet specified here will be ignored because it is impossible to determine which currency it is)
curl -v --user admin:admin --data-binary '{"id":null,"method":"createauxblock","params":["my2dxGb5jz43ktwGxg2doUaEb9WhZ9PQ7K"]}' -H 'content-type: application/json' http://localhost:8999/

# Submit a task
curl -v --user admin:admin --data-binary '{"id":1,"method":"submitauxblock","params":["6c8adaefa07ab5ff14ddff1b8e2bae4ecfc59ef0a985064bd202565106ff054b","02000000010000000000000000000000000000000000000000000000000000000000000000ffffffff4b039ccd09041b96485b742f4254432e434f4d2ffabe6d6d6c8adaefa07ab5ff14ddff1b8e2bae4ecfc59ef0a985064bd202565106ff054b020000004204cb9a010388711000000000000000ffffffff0200000000000000001976a914c0174e89bd93eacd1d5a1af4ba1802d412afc08688ac0000000000000000266a24aa21a9ede2f61c3f71d1defd3fa999dfa36953755c690689799962b48bebd836974e8cf9000000002d6009ef30ae316bcebe42ea7f4feaf995fb34211aa80b9835e06b0388769ce6000000000000000000000000002075cc0a4e259833d348dd282c00a61ab112bea0e02d1ac85e4773d08a01b87b3f2dc921f1fd927d473649cbb7115debb95de77455401566d56b12a94cbfca8dff1b96485bffff7f201b96485b"]}' -H 'content-type: application/json' http://localhost:8999/
```

#### Get tasks

The format of the returned result is as follows:
```js
{
    "id": "1",
    "result": {
        "hash": "9e077526b9e82040ec82492993d6e1d25c92ce572d03eb1caa6d3b868a558a32", // Combined mining block hash (actually the merkle root of multiple block hashes)
        "chainid": 1, // Fake chain id, always 1
        "previousblockhash": "949a1539fa4ac733d651f6709967d541374e3e23f4422ea6ac2bf925e385807a", // The parent block hash of the current block of the first blockchain
        "coinbasevalue": 5000000000, // The current block reward of the first blockchain
        "bits": "207fffff", // The bits corresponding to the smallest difficulty in multiple blockchains
        "height": 123, // The current block height of the first blockchain
        "_target": "0000000000000000000000000000000000000000000000000000000000ffff7f", // The target corresponding to the above bits
        "merkle_size": 2, // size of merged merkle tree
        "merkle_nonce": 2596996162 // Random value used to determine the position of each blockchain on the merkle tree
    },
    "error": null
}
```

The format is similar to the return value of the same RPC for Namecoin, but with two more fields: `merkle_size` and `merkle_nonce`.

In order to correctly embed the multi-currency merged mining tag in the main chain (Bitcoin), the mining pool must adapt, identify these two fields and put them in the [coinbase transaction merged mining tag](https://en.bitcoin.it/wiki/Merged_mining_specification#Merged_mining_coinbase)。


#### Submit a task

If the submitted workload meets the difficulty of at least one blockchain, return:

```
{"id":1,"result":true,"error":null}
````

Otherwise, return

```
{"id":1,"result":null,"error":{"code":400,"message":"high-hash"}}
````

If other errors occur, such as `block hash` is not found, or the format of `aux pow` is incorrect, etc., a corresponding error message will be returned, the format is similar to the above `400 high-hash`.
E.g：

```
{"id":1,"result":null,"error":{"code":400,"message":"cannot found blockHash d725af6c2243cdc8eb1180f72f820b8692f360ec1a2d87df0ba0c7c1c61f2c95 from AuxPowData 02000000010000000000000000000000000000000000000000000000000000000000000000ffffffff4b039ccd09041b96485b742f4254432e434f4d2ffabe6d6d6c8adaefa07ab5ff14ddff1b8e2bae4ecfc59ef0a985064bd202565106ff054b020000004204cb9a010388711000000000000000ffffffff0200000000000000001976a914c0174e89bd93eacd1d5a1af4ba1802d412afc08688ac0000000000000000266a24aa21a9ede2f61c3f71d1defd3fa999dfa36953755c690689799962b48bebd836974e8cf9000000002d6009ef30ae316bcebe42ea7f4feaf995fb34211aa80b9835e06b0388769ce6000000000000000000000000002075cc0a4e259833d348dd282c00a61ab112bea0e02d1ac85e4773d08a01b87b3f2dc921f1fd927d473649cbb7115debb95de77455401566d56b12a94cbfca8dff1b96485bffff7f301b96485b"}}
```

Note that the fact that this RPC returns `true` does not mean that the workload is actually accepted by at least one blockchain. Whether the submission is successful or not depends on the log of this program.


### TODO

* Write the block records of each blockchain into the database.
