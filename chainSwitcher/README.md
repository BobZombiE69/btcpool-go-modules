# Chain Switcher

According to the currency price information provided by the HTTP API, send the currency switching command to Kafka

## HTTP interface

The interface should return JSON like this:

```
{
    "algorithms": {
        "SHA256": {
            "coins": [
                "BCH",
                "BTC"
            ]
        }
    }
}
```

Among them: `Coins` is a recommended currency for excavation, sorted from high to low according to income.

## Construct
```
go get github.com/segmentio/kafka-go
go get github.com/golang/snappy
go get github.com/go-sql-driver/mysql
go get github.com/golang/glog
go build
```

## run
```
cp config.default.json config.json
./chainSwitcher --config config.json --logtostderr
```

# Docker

## Construct
```
cd btcpool-go-modules/chainSwitcher
docker build -t btcpool-chain-switcher -f Dockerfile ..
```

## run
```
docker run -it --rm --network=host \
    -e KafkaBrokers=127.0.0.1:9092,127.0.0.2:9092,127.0.0.3:9092 \
    -e KafkaControllerTopic=BtcManController \
    -e KafkaProcessorTopic=BtcManProcessor \
    -e Algorithm=SHA256 \
    -e ChainDispatchAPI=http://127.0.0.1:8000/chain-dispatch.php \
    -e FailSafeChain=btc \
    -e ChainNameMap='{"BTC":"btc","BCH":"bcc"}' \
    -e MySQLConnStr="root:root@/bpool_local_db" \
    btcpool-chain-switcher -logtostderr -v 2

# All parameters:
docker run -it --rm --network=host \
    -e KafkaBrokers=127.0.0.1:9092,127.0.0.2:9092,127.0.0.3:9092 \
    -e KafkaControllerTopic=BtcManController \
    -e KafkaProcessorTopic=BtcManProcessor \
    -e Algorithm=SHA256 \
    -e ChainDispatchAPI=http://127.0.0.1:8000/chain-dispatch.php \
    -e SwitchIntervalSeconds=60 \
    -e FailSafeChain=btc \
    -e FailSafeSeconds=600 \
    -e ChainNameMap='{"BTC":"btc","BCH":"bcc","BSV":"bsv"}' \
    -e MySQLConnStr="root:root@tcp(localhost:3306)/bpool_local_db" \
    -e MySQLTable="chain_switcher_record" \
    \
    -e ChainLimits_bcc_MaxHashrate="100P" \
    -e ChainLimits_bcc_MySQLConnStr="root:root@tcp(localhost:3306)/bcc_local_db" \
    -e ChainLimits_bcc_MySQLTable="mining_workers" \
    \
    -e ChainLimits_bsv_MaxHashrate="50P" \
    -e ChainLimits_bsv_MySQLConnStr="root:root@tcp(localhost:3306)/bsv_local_db" \
    -e ChainLimits_bsv_MySQLTable="mining_workers" \
    \
    -e RecordLifetime="60" \
    btcpool-chain-switcher -logtostderr -v 2

# Guardian
docker run -it --name chain-switcher --network=host --restart always -d \
    -e KafkaBrokers=127.0.0.1:9092,127.0.0.2:9092,127.0.0.3:9092 \
    -e KafkaControllerTopic=BtcManController \
    -e KafkaProcessorTopic=BtcManProcessor \
    -e Algorithm=SHA256 \
    -e ChainDispatchAPI=http://127.0.0.1:8000/chain-dispatch.php \
    -e FailSafeChain=btc \
    -e ChainNameMap='{"BTC":"btc","BCH":"bcc"}' \
    -e MySQLConnStr="root:root@/bpool_local_db" \
    btcpool-chain-switcher -logtostderr -v 2
```

## database change
The program will automatically try to create the following data table:
```
CREATE TABLE IF NOT EXISTS `<configData.MySQL.Table的值>`(
    id bigint(20) NOT NULL AUTO_INCREMENT,
    algorithm varchar(255) NOT NULL,
    prev_chain varchar(255) NOT NULL,
    curr_chain varchar(255) NOT NULL,
    api_result text NOT NULL,
    created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id)
)
```
