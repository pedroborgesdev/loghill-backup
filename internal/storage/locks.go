package storage

import "sync"

type LockManager struct{ locks sync.Map }

func (m *LockManager) Get(id string) *sync.RWMutex {
	v, _ := m.locks.LoadOrStore(id, &sync.RWMutex{})
	return v.(*sync.RWMutex)
}
