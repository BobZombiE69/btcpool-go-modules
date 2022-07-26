package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/snappy"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/segmentio/kafka-go/snappy"
)

// MySQLInfo Mysql connection information
type MySQLInfo struct {
	ConnStr string
	Table   string
}

// ChainLimit Blockchain computing power restriction
type ChainLimit struct {
	MaxHashrate string
	MySQL       MySQLInfo

	name         string  // internal use
	hashrate     float64 // Limit's floating point number represents, internal use, avoid repeated analysis of string
	hashrateBase float64 //The computing power coefficient corresponding to the currency, internal use, avoid repeated analysis configuration
}

// ChainSwitcherConfig Program configuration
type ChainSwitcherConfig struct {
	Kafka struct {
		Brokers         []string
		ControllerTopic string
		ProcessorTopic  string
	}
	Algorithm             string
	ChainDispatchAPI      string
	SwitchIntervalSeconds time.Duration
	FailSafeChain         string
	FailSafeSeconds       time.Duration
	ChainNameMap          map[string]string
	MySQL                 MySQLInfo
	ChainLimits           map[string]ChainLimit
	RecordLifetime        uint64
}

// ChainRecord HTTP APICurrency record
type ChainRecord struct {
	Coins []string `json:"coins"`
}

// ChainDispatchRecord HTTP APIresponse
type ChainDispatchRecord struct {
	Algorithms map[string]ChainRecord `json:"algorithms"`
}

// KafkaMessage Received message structure in Kafka
type KafkaMessage struct {
	ID                  interface{} `json:"id"`
	Type                string      `json:"type"`
	Action              string      `json:"action"`
	CreatedAt           string      `json:"created_at"`
	NewChainName        string      `json:"new_chain_name"`
	OldChainName        string      `json:"old_chain_name"`
	Result              bool        `json:"result"`
	ServerID            int         `json:"server_id"`
	SwitchedConnections int         `json:"switched_connections"`
	SwitchedUsers       int         `json:"switched_users"`
	Host                struct {
		Hostname string              `json:"hostname"`
		IP       map[string][]string `json:"ip"`
	} `json:"host"`
}

// KafkaCommand Structure of messages sent in Kafka
type KafkaCommand struct {
	ID        interface{} `json:"id"`
	Type      string      `json:"type"`
	Action    string      `json:"action"`
	CreatedAt string      `json:"created_at"`
	ChainName string      `json:"chain_name"`
}

// ActionFailSafeSwitch api_result logged when the API fails over to the default currency
type ActionFailSafeSwitch struct {
	Action         string `json:"action"`
	LastUpdateTime int64  `json:"last_update_time"`
	CurrentTime    int64  `json:"current_time"`
	OldChainName   string `json:"old_chain_name"`
	NewChainName   string `json:"new_chain_name"`
}

// Configuration Data
var configData *ChainSwitcherConfig

var updateTime int64
var currentChainName string

var controllerProducer *kafka.Writer
var processorConsumer *kafka.Reader
var commandID uint64

var insertStmt *sql.Stmt
var mysqlConn *sql.DB

func main() {
	// Parse command line arguments
	configFilePath := flag.String("config", "./config.json", "Path of config file")
	flag.Parse()

	// read configuration file
	configJSON, err := ioutil.ReadFile(*configFilePath)

	if err != nil {
		glog.Fatal("read config failed: ", err)
		return
	}

	configData = new(ChainSwitcherConfig)
	err = json.Unmarshal(configJSON, configData)

	if err != nil {
		glog.Fatal("parse config failed: ", err)
		return
	}

	// Verify configuration
	for chain, limit := range configData.ChainLimits {
		limit.hashrate, err = parseHashrate(limit.MaxHashrate)
		if err != nil {
			glog.Fatal("wrong limit number of chain ", chain, ": ", limit.MaxHashrate, ", ", err)
			return
		}

		limit.hashrateBase = getHashrateBase(chain)
		if limit.hashrateBase <= 0 {
			glog.Fatal("unknown hashrate base of chain ", chain, ": ", limit.hashrateBase)
			return
		}

		limit.name = chain
		configData.ChainLimits[chain] = limit

		glog.Info("chain ", limit.name, " max hashrate: ", formatHashrate(limit.hashrate))
	}
	if configData.RecordLifetime == 0 {
		configData.RecordLifetime = 60
	}

	processorConsumer = kafka.NewReader(kafka.ReaderConfig{
		Brokers:   configData.Kafka.Brokers,
		Topic:     configData.Kafka.ProcessorTopic,
		Partition: 0,
		MinBytes:  128,  // 128B
		MaxBytes:  10e6, // 10MB
	})

	controllerProducer = kafka.NewWriter(kafka.WriterConfig{
		Brokers:          configData.Kafka.Brokers,
		Topic:            configData.Kafka.ControllerTopic,
		Balancer:         &kafka.LeastBytes{},
		CompressionCodec: snappy.NewCompressionCodec(),
	})

	initMySQL()
	go failSafe()
	go readResponse()
	updateChain()
}

func initMySQL() {
	var err error

	glog.Info("connecting to MySQL...")
	mysqlConn, err = sql.Open("mysql", configData.MySQL.ConnStr)
	if err != nil {
		glog.Fatal("mysql error: ", err)
		return
	}

	err = mysqlConn.Ping()
	if err != nil {
		glog.Fatal("mysql error: ", err.Error())
		return
	}

	mysqlConn.Exec("CREATE TABLE IF NOT EXISTS `" + configData.MySQL.Table + "`(" + `
		id bigint(20) NOT NULL AUTO_INCREMENT,
		algorithm varchar(255) NOT NULL,
		prev_chain varchar(255) NOT NULL,
		curr_chain varchar(255) NOT NULL,
		api_result text NOT NULL,
		created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id)
		)
	`)

	insertStmt, err = mysqlConn.Prepare("INSERT INTO `" + configData.MySQL.Table +
		"`(algorithm,prev_chain,curr_chain,api_result) VALUES(?,?,?,?)")
	if err != nil {
		glog.Fatal("mysql error: ", err.Error())
		return
	}
}

func getHashrate(chainLimit ChainLimit) (hashrate5m float64, userNum int64, err error) {
	glog.Info("connecting to MySQL of chain ", chainLimit.name, "...")
	conn, err := sql.Open("mysql", chainLimit.MySQL.ConnStr)
	if err != nil {
		return
	}

	sql := "SELECT sum(accept_5m), sum(1) FROM `" + chainLimit.MySQL.Table + "` WHERE " +
		"worker_id = 0 AND " +
		"unix_timestamp() - unix_timestamp(updated_at) < " + strconv.FormatUint(configData.RecordLifetime, 10)
	glog.V(5).Info("SQL: ", sql)
	rows, err := conn.Query(sql)
	if err != nil {
		return
	}

	if !rows.Next() {
		return
	}

	rows.Scan(&hashrate5m, &userNum)
	// hashrate5m = share * base / time
	hashrate5m *= chainLimit.hashrateBase / 300
	return
}

func failSafe() {
	for {
		time.Sleep(configData.FailSafeSeconds * time.Second)

		now := time.Now().Unix()
		if updateTime+int64(configData.FailSafeSeconds) < now {
			oldChainName := currentChainName
			currentChainName = configData.FailSafeChain

			glog.Info("Fail Safe Switch: ", oldChainName, " -> ", currentChainName,
				", lastUpdateTime: ", time.Unix(updateTime, 0).UTC().Format("2006-01-02 15:04:05"),
				", currentTime: ", time.Unix(now, 0).UTC().Format("2006-01-02 15:04:05"))
			sendCurrentChainToKafka()

			apiResult := ActionFailSafeSwitch{
				"fail_safe_switch",
				updateTime,
				now,
				oldChainName,
				currentChainName}
			bytes, _ := json.Marshal(apiResult)
			_, err := insertStmt.Exec(configData.Algorithm, oldChainName, currentChainName, bytes)
			if err != nil {
				glog.Fatal("mysql error: ", err.Error())
				return
			}

			updateTime = now
		}
	}
}

func sendCurrentChainToKafka() {
	commandID++
	command := KafkaCommand{
		commandID,
		"sserver_cmd",
		"auto_switch_chain",
		time.Now().UTC().Format("2006-01-02 15:04:05"),
		currentChainName}
	bytes, _ := json.Marshal(command)
	controllerProducer.WriteMessages(context.Background(), kafka.Message{Value: []byte(bytes)})

	glog.Info("Send to Kafka, id: ", command.ID,
		", created_at: ", command.CreatedAt,
		", type: ", command.Type,
		", action: ", command.Action,
		", chain_name: ", command.ChainName)
}

func updateChain() {
	for {
		updateCurrentChain()
		if currentChainName != "" {
			sendCurrentChainToKafka()
		}

		time.Sleep(configData.SwitchIntervalSeconds * time.Second)
	}
}

func updateCurrentChain() {
	oldChainName := currentChainName

	glog.Info("HTTP GET ", configData.ChainDispatchAPI)
	response, err := http.Get(configData.ChainDispatchAPI)
	if err != nil {
		glog.Error("HTTP Request Failed: ", err)
		return
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		glog.Error("HTTP Fetch Body Failed: ", err)
		return
	}

	chainDispatchRecord := new(ChainDispatchRecord)
	err = json.Unmarshal(body, chainDispatchRecord)
	if err != nil {
		glog.Error("Parse Result Failed: ", err)
		return
	}

	algorithms, ok := chainDispatchRecord.Algorithms[configData.Algorithm]
	if !ok {
		glog.Error("Cannot find algorithm ", configData.Algorithm, ", json: ", string(body))
		return
	}

	bestChain := configData.FailSafeChain
	for _, coin := range algorithms.Coins {
		chainName, ok := configData.ChainNameMap[coin]
		if ok {
			if limit, ok := configData.ChainLimits[chainName]; ok {
				hashrate, userNum, err := getHashrate(limit)
				if err == nil {
					if hashrate < limit.hashrate {
						glog.Info("chain ", limit.name, " (hashrate: ", formatHashrate(hashrate),
							") < (limit: ", formatHashrate(limit.hashrate), "), ",
							userNum, " users, selected")
						bestChain = chainName
						break
					} else {
						glog.Info("chain ", limit.name, " (hashrate: ", formatHashrate(hashrate),
							") >= (limit: ", formatHashrate(limit.hashrate), "), ",
							userNum, " users,  ignored")
					}
				} else {
					glog.Error("get hashrate of chain ", limit.name, " failed: ", err)
				}
			} else {
				bestChain = chainName
				break
			}
		}
	}

	if bestChain != "" {
		currentChainName = bestChain
		updateTime = time.Now().Unix()
	}

	if oldChainName != currentChainName {
		glog.Info("Best Chain Changed: ", oldChainName, " -> ", bestChain)
		_, err := insertStmt.Exec(configData.Algorithm, oldChainName, currentChainName, body)
		if err != nil {
			glog.Fatal("mysql error: ", err.Error())
			return
		}
	} else {
		glog.Info("Best Chain not Changed: ", bestChain)
	}
}

func readResponse() {
	processorConsumer.SetOffset(kafka.LastOffset)
	for {
		m, err := processorConsumer.ReadMessage(context.Background())
		if err != nil {
			glog.Error("read kafka failed: ", err)
			continue
		}
		response := new(KafkaMessage)
		err = json.Unmarshal(m.Value, response)
		if err != nil {
			glog.Error("Parse Result Failed: ", err)
			continue
		}

		if response.Type == "sserver_response" && response.Action == "auto_switch_chain" {
			glog.Info("Server Response, id: ", response.ID,
				", created_at: ", response.CreatedAt,
				", server_id: ", response.ServerID,
				", result: ", response.Result,
				", old_chain_name: ", response.OldChainName,
				", new_chain_name: ", response.NewChainName,
				", switched_users: ", response.SwitchedUsers,
				", switched_connections: ", response.SwitchedConnections)
			continue
		}

		if response.Type == "sserver_notify" && response.Action == "online" {
			glog.Info("Server Online, ",
				", created_at: ", response.CreatedAt,
				", server_id: ", response.ServerID,
				", hostname: ", response.Host.Hostname,
				", ip: ", response.Host.IP)
			sendCurrentChainToKafka()
			continue
		}
	}
}
