package fsm_test

import (
	"testing"

	"github.com/AkZcH/herald/internal/fsm"
)

func TestValidTransitions(t *testing.T) {
	f := fsm.New()

	if err := f.Transition(fsm.StateRollbackPending); err != nil {
		t.Errorf("expected ACTIVE → ROLLBACK_PENDING to be valid: %v", err)
	}
	if err := f.Transition(fsm.StateCommitting); err != nil {
		t.Errorf("expected ROLLBACK_PENDING → COMMITTING to be valid: %v", err)
	}
	if err := f.Transition(fsm.StateRolledBack); err != nil {
		t.Errorf("expected COMMITTING → ROLLED_BACK to be valid: %v", err)
	}
}

func TestInvalidTransitions(t *testing.T) {
	f := fsm.New()

	if err := f.Transition(fsm.StateCommitting); err == nil {
		t.Error("expected ACTIVE → COMMITTING to be invalid")
	}
	if err := f.Transition(fsm.StateRolledBack); err == nil {
		t.Error("expected ACTIVE → ROLLED_BACK to be invalid")
	}
}

func TestConcurrentRollbackGuard(t *testing.T) {
	f := fsm.New()

	if err := f.Transition(fsm.StateRollbackPending); err != nil {
		t.Fatalf("first transition failed: %v", err)
	}

	if err := f.Transition(fsm.StateRollbackPending); err == nil {
		t.Error("expected concurrent rollback to be rejected")
	}
}

func TestRollbackAbort(t *testing.T) {
	f := fsm.New()

	f.Transition(fsm.StateRollbackPending)
	if err := f.Transition(fsm.StateActive); err != nil {
		t.Errorf("expected ROLLBACK_PENDING → ACTIVE (abort) to be valid: %v", err)
	}
}