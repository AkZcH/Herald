package fsm

import (
	"fmt"
	"sync"
)

type RollbackState string

const (
	StateActive          RollbackState = "ACTIVE"
	StateRollbackPending RollbackState = "ROLLBACK_PENDING"
	StateCommitting      RollbackState = "COMMITTING"
	StateRolledBack      RollbackState = "ROLLED_BACK"
)

type RollbackFSM struct {
	mu    sync.Mutex
	state RollbackState
}

func New() *RollbackFSM {
	return &RollbackFSM{state: StateActive}
}

func (f *RollbackFSM) State() RollbackState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

func (f *RollbackFSM) Transition(next RollbackState) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.validTransition(f.state, next) {
		return fmt.Errorf("invalid transition: %s → %s", f.state, next)
	}

	f.state = next
	return nil
}

func (f *RollbackFSM) validTransition(from, to RollbackState) bool {
	transitions := map[RollbackState][]RollbackState{
		StateActive:          {StateRollbackPending},
		StateRollbackPending: {StateCommitting, StateActive},
		StateCommitting:      {StateActive, StateRolledBack},
		StateRolledBack:      {},
	}

	allowed, ok := transitions[from]
	if !ok {
		return false
	}

	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}