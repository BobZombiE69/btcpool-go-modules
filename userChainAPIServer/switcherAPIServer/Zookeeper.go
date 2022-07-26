package switcherapiserver

import (
	"strings"

	"github.com/golang/glog"
	"github.com/samuel/go-zookeeper/zk"
)

// Create Zookeeper recursively Node
func createZookeeperPath(path string) error {
	pathTrimmed := strings.Trim(path, "/")
	dirs := strings.Split(pathTrimmed, "/")

	currPath := ""

	for _, dir := range dirs {
		currPath += "/" + dir

		// see if the key exists
		exists, _, err := zookeeperConn.Exists(currPath)

		if err != nil {
			return err
		}

		// already exists, no need to create
		if exists {
			continue
		}

		// does not exist, create
		_, err = zookeeperConn.Create(currPath, []byte{}, 0, zk.WorldACL(zk.PermAll))

		if err != nil {
			// Then see if the key exists (the key may have been created by another thread)
			exists, _, _ = zookeeperConn.Exists(currPath)
			if exists {
				continue
			}
			// key does not exist, return error
			return err
		}

		glog.Info("Created zookeeper path: ", currPath)
	}

	return nil
}
