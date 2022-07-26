package main

import (
	"errors"
	"strconv"
	"sync"

	"github.com/willf/bitset"
)

//////////////////////////////// SessionIDManager //////////////////////////////

// SessionIDManager Thread-safe session ID manager
type SessionIDManager struct {
	//
	//  SESSION ID: UINT32
	//
	//   xxxxxxxx     xxxxxxxx xxxxxxxx xxxxxxxx
	//  ----------    --------------------------
	//  server ID         session index id
	//   [1, 255]        range: [0, MaxValidSessionID]
	//
	serverID   uint32
	sessionIDs *bitset.BitSet

	count         uint32 // how many ids are used now
	allocIDx      uint32
	allocInterval uint32
	lock          sync.Mutex

	indexBits uint8 // bits of session index id
	// SessionIDMask session ID mask, used to separate serverID and sessionID
	// It is also the maximum value that the sessionID part can reach
	sessionIDMask uint32
}

// NewSessionIDManager Create a session ID manager instance
func NewSessionIDManager(serverID uint8, indexBits uint8) (manager *SessionIDManager, err error) {
	if indexBits > 24 {
		err = errors.New("indexBits should not > 24, but it = " + strconv.Itoa(int(indexBits)))
		return
	}
	if serverID == 0 {
		err = errors.New("serverID not set (serverID = 0)")
		return
	}

	manager = new(SessionIDManager)

	manager.sessionIDMask = (1 << indexBits) - 1

	manager.serverID = uint32(serverID) << indexBits
	manager.sessionIDs = bitset.New(uint(manager.sessionIDMask + 1))
	manager.count = 0
	// Set an initial value different from sserver to catch session ID inconsistencies early
	// (server forgot to enable the WORK_WITH_STRATUM_SWITCHER compile option)
	manager.allocIDx = 128
	manager.allocInterval = 0

	manager.sessionIDs.ClearAll()
	return
}

// setAllocInterval sets the interval for allocating id
// This feature temporarily reserves more mining space for sessions without DoS risk
// (Currently for compatibility with NiceHash Ethereum client)
func (manager *SessionIDManager) setAllocInterval(interval uint32) {
	manager.allocInterval = interval
}

// isFull Determine whether the session ID is full (internal use, not locked)
func (manager *SessionIDManager) isFullWithoutLock() bool {
	return (manager.count > manager.sessionIDMask)
}

// IsFull Determine if the session ID is full
func (manager *SessionIDManager) IsFull() bool {
	defer manager.lock.Unlock()
	manager.lock.Lock()

	return manager.isFullWithoutLock()
}

// AllocSessionID Assign a session ID to the caller
func (manager *SessionIDManager) AllocSessionID() (sessionID uint32, err error) {
	defer manager.lock.Unlock()
	manager.lock.Lock()

	if manager.isFullWithoutLock() {
		sessionID = manager.sessionIDMask
		err = ErrSessionIDFull
		return
	}

	// find an empty bit
	for manager.sessionIDs.Test(uint(manager.allocIDx)) {
		manager.allocIDx = (manager.allocIDx + 1) & manager.sessionIDMask
	}

	// set to true
	manager.sessionIDs.Set(uint(manager.allocIDx))
	manager.count++

	sessionID = manager.serverID | manager.allocIDx
	err = nil
	manager.allocIDx = (manager.allocIDx + manager.allocInterval) & manager.sessionIDMask
	return
}

// ResumeSessionID Restore previous session ID
func (manager *SessionIDManager) ResumeSessionID(sessionID uint32) (err error) {
	defer manager.lock.Unlock()
	manager.lock.Lock()

	idx := sessionID & manager.sessionIDMask

	// test if the bit be empty
	if manager.sessionIDs.Test(uint(idx)) {
		err = ErrSessionIDOccupied
		return
	}

	// set to true
	manager.sessionIDs.Set(uint(idx))
	manager.count++

	if manager.allocIDx <= idx {
		manager.allocIDx = idx + manager.allocInterval
	}

	err = nil
	return
}

// FreeSessionID Release the session ID held by the caller
func (manager *SessionIDManager) FreeSessionID(sessionID uint32) {
	defer manager.lock.Unlock()
	manager.lock.Lock()

	idx := sessionID & manager.sessionIDMask

	if !manager.sessionIDs.Test(uint(idx)) {
		// ID is not allocated, no need to free
		return
	}

	manager.sessionIDs.Clear(uint(idx))
	manager.count--
}
