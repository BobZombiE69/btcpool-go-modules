package main

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/samuel/go-zookeeper/zk"
)

// zookeeper connection timeout
const zookeeperConnectingTimeoutSeconds = 60

// Zookeeper connection deactivation timeout
const zookeeperConnAliveTimeout = 5

// NodeWatcherChannels Node monitor's channel
type NodeWatcherChannels map[uint32]chan zk.Event

// NodeWatcher Node monitor
type NodeWatcher struct {
	// Zookeeper Manager
	zookeeperManager *ZookeeperManager
	// The path of the monitored node
	nodePath string
	// The current value of the monitored node
	nodeValue []byte
	// Monitored Zookeeper events
	zkWatchEvent <-chan zk.Event
	// Node monitor's channel
	watcherChannels NodeWatcherChannels
}

// NewNodeWatcher New Node Monitor
func NewNodeWatcher(zookeeperManager *ZookeeperManager) *NodeWatcher {
	watcher := new(NodeWatcher)
	watcher.zookeeperManager = zookeeperManager
	watcher.watcherChannels = make(NodeWatcherChannels)
	return watcher
}

// Run 开始监控
func (watcher *NodeWatcher) Run() {
	go func() {
		event := <-watcher.zkWatchEvent

		watcher.zookeeperManager.lock.Lock()
		defer watcher.zookeeperManager.lock.Unlock()

		for _, eventChan := range watcher.watcherChannels {
			eventChan <- event
			close(eventChan)
		}

		watcher.zookeeperManager.removeNodeWatcher(watcher)
	}()
}

// NodeWatcherMap Zookeeper监控器Map
type NodeWatcherMap map[string]*NodeWatcher

// ZookeeperManager Zookeeper管理器
type ZookeeperManager struct {
	// The lock added when modifying watcherMap
	lock sync.Mutex
	// 监控器Map
	watcherMap NodeWatcherMap
	// Zookeeper connection
	zookeeperConn *zk.Conn
}

// NewZookeeperManager New Zookeeper Manager
func NewZookeeperManager(brokers []string) (manager *ZookeeperManager, err error) {
	manager = new(ZookeeperManager)
	manager.watcherMap = make(NodeWatcherMap)

	// Establish a connection to the Zookeeper cluster
	var event <-chan zk.Event
	manager.zookeeperConn, event, err = zk.Connect(brokers, time.Duration(zookeeperConnAliveTimeout)*time.Second)
	if err != nil {
		return
	}

	zkConnected := make(chan bool, 1)

	go func() {
		glog.Info("Zookeeper: waiting for connecting to ", brokers, "...")
		for {
			e := <-event
			glog.Info("Zookeeper: ", e)

			if e.State == zk.StateConnected {
				zkConnected <- true
				return
			}
		}
	}()

	select {
	case <-zkConnected:
		break
	case <-time.After(zookeeperConnectingTimeoutSeconds * time.Second):
		err = errors.New("Zookeeper: connecting timeout")
		break
	}

	return
}

// removeNodeWatcher remove monitor node
func (manager *ZookeeperManager) removeNodeWatcher(watcher *NodeWatcher) {
	delete(manager.watcherMap, watcher.nodePath)
	if glog.V(3) {
		glog.Info("Zookeeper: release NodeWatcher: ", watcher.nodePath)
	}
}

// GetW Get the value of the Zookeeper node and set up monitoring
func (manager *ZookeeperManager) GetW(path string, sessionID uint32) (value []byte, event <-chan zk.Event, err error) {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	watcher, exists := manager.watcherMap[path]

	if !exists {
		watcher = NewNodeWatcher(manager)
		watcher.nodePath = path
		watcher.nodeValue, _, watcher.zkWatchEvent, err = manager.zookeeperConn.GetW(path)

		if err != nil {
			return
		}

		manager.watcherMap[path] = watcher
		if glog.V(3) {
			glog.Info("Zookeeper: add NodeWatcher: ", path)
		}

		defer watcher.Run()
	}

	eventChan := make(chan zk.Event, 1)
	watcher.watcherChannels[sessionID] = eventChan
	if glog.V(3) {
		glog.Info("Zookeeper: add WatcherChannel: ", path, "; ", Uint32ToHex(sessionID))
	}

	value = watcher.nodeValue
	event = eventChan
	return
}

// Create Create a Zookeeper stanza点
func (manager *ZookeeperManager) Create(path string, data []byte) (err error) {
	_, err = manager.zookeeperConn.Create(path, data, 0, zk.WorldACL(zk.PermAll))
	return
}

// ReleaseW release monitoring
func (manager *ZookeeperManager) ReleaseW(path string, sessionID uint32) {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	watcher, exists := manager.watcherMap[path]

	if !exists {
		return
	}

	eventChan, exists := watcher.watcherChannels[sessionID]

	if !exists {
		return
	}

	close(eventChan)
	delete(watcher.watcherChannels, sessionID)
	if glog.V(3) {
		glog.Info("Zookeeper: release WatcherChannel: ", path, "; ", Uint32ToHex(sessionID))
	}

	// The code of go-zookeeper shows that its watcher will only close and release after receiving the event,
	// Therefore, removing NodaWatcher here does not free the watcher in go-zookeeper,
	// Moreover, repeatedly opening new watchers will cause a large number of watchers to be generated at go-zookeeper and memory leaks.
	// Therefore, NodeWatcher is no longer automatically released here. NodeWatcher is only released after receiving zookeeper events.
	/*
		if len(watcher.watcherChannels) == 0 {
			manager.removeNodeWatcher(watcher)
		}
	*/
}

// Create Zookeeper recursively Node
func (manager *ZookeeperManager) createZookeeperPath(path string) error {
	pathTrimmed := strings.Trim(path, "/")
	dirs := strings.Split(pathTrimmed, "/")

	currPath := ""

	for _, dir := range dirs {
		currPath += "/" + dir

		// see if the key exists
		exists, _, err := manager.zookeeperConn.Exists(currPath)

		if err != nil {
			return err
		}

		// already exists, no need to create
		if exists {
			continue
		}

		// does not exist, create
		_, err = manager.zookeeperConn.Create(currPath, []byte{}, 0, zk.WorldACL(zk.PermAll))

		if err != nil {
			return err
		}

		glog.Info("Created zookeeper path: ", currPath)
	}

	return nil
}
