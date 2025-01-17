package main

import (
	"errors"
	"os"

	"github.com/golang/glog"
)

// Variables that hold runtime state files
const runtimeFilePath = "./runtime.json"

// Upgradable Upgrading StratumSwitcher processes without downtime
type Upgradable struct {
	sessionManager *StratumSessionManager
}

// NewUpgradable Create an Upgradable object
func NewUpgradable(sessionManager *StratumSessionManager) (upgradable *Upgradable) {
	upgradable = new(Upgradable)
	upgradable.sessionManager = sessionManager
	return
}

// Upgrade StratumSwitcher process
func (upgradable *Upgradable) upgradeStratumSwitcher() (err error) {
	glog.Info("Upgrading...")

	var runtimeData RuntimeData
	runtimeData.Action = "upgrade"
	runtimeData.ServerID = upgradable.sessionManager.serverID

	upgradable.sessionManager.lock.Lock()
	err = func() error {
		for _, session := range upgradable.sessionManager.sessions {
			var sessionData StratumSessionData

			sessionData.SessionID = session.sessionID
			sessionData.MiningCoin = session.miningCoin
			sessionData.StratumSubscribeRequest = session.stratumSubscribeRequest
			sessionData.StratumAuthorizeRequest = session.stratumAuthorizeRequest
			sessionData.VersionMask = session.versionMask

			sessionData.ClientConnFD, err = getConnFd(session.clientConn)
			if err != nil {
				return errors.New("getConnFd Failed: " + err.Error())
			}

			sessionData.ServerConnFD, err = getConnFd(session.serverConn)
			if err != nil {
				return errors.New("getConnFd Failed: " + err.Error())
			}

			err = setNoCloseOnExec(sessionData.ClientConnFD)
			if err != nil {
				return errors.New("setNoCloseOnExec Failed: " + err.Error())
			}

			err = setNoCloseOnExec(sessionData.ServerConnFD)
			if err != nil {
				return errors.New("setNoCloseOnExec Failed: " + err.Error())
			}

			runtimeData.SessionDatas = append(runtimeData.SessionDatas, sessionData)
		}

		return nil
	}()
	upgradable.sessionManager.lock.Unlock()
	if err != nil {
		return
	}

	err = runtimeData.SaveToFile(runtimeFilePath)
	if err != nil {
		return
	}

	upgradable.sessionManager.zookeeperManager.zookeeperConn.Close()

	var args []string
	for _, arg := range os.Args[1:] {
		if len(arg) < 9 || arg[0:9] != "-runtime=" {
			args = append(args, arg)
		}
	}
	args = append(args, "-runtime="+runtimeFilePath)

	err = execNewBin(os.Args[0], args)
	return
}
