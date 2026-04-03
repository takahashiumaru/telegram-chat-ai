package state

import (
	"encoding/json"
	"os"
	"sync"
	"kaguya-telegram/internal/config"
	"kaguya-telegram/internal/model"
)

type StateManager struct {
	mu           sync.RWMutex
	lastReported map[string]model.State
}

func NewStateManager() *StateManager {
	sm := &StateManager{
		lastReported: make(map[string]model.State),
	}
	sm.Load()
	return sm
}

func (sm *StateManager) Load() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	data, err := os.ReadFile(config.StateFilePath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &sm.lastReported)
}

func (sm *StateManager) Save() {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	data, _ := json.Marshal(sm.lastReported)
	_ = os.WriteFile(config.StateFilePath, data, 0644)
}

func (sm *StateManager) Get(projectID string) (model.State, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.lastReported[projectID]
	return s, ok
}

func (sm *StateManager) Set(projectID string, state model.State) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.lastReported[projectID] = state
}
