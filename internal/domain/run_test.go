package domain

import (
	"errors"
	"testing"
)

func TestRunStateTransitions(t *testing.T) {
	valid := [][2]RunState{{RunQueued, RunPreparingContext}, {RunPreparingContext, RunWaitingForModel}, {RunWaitingForModel, RunUsingTool}, {RunUsingTool, RunWaitingForModel}, {RunWaitingForModel, RunValidating}, {RunValidating, RunAwaitingApproval}, {RunAwaitingApproval, RunCompleted}}
	for _, transition := range valid {
		if err := ValidateRunTransition(transition[0], transition[1]); err != nil {
			t.Errorf("transition %s -> %s: %v", transition[0], transition[1], err)
		}
	}
	if err := ValidateRunTransition(RunCompleted, RunPreparingContext); err == nil {
		t.Fatal("terminal run transitioned back to active")
	}
	if err := ValidateRunTransition(RunWaitingForModel, RunCancelled); err != nil {
		t.Fatalf("active run cancellation rejected: %v", err)
	}
}

func TestRunBudget(t *testing.T) {
	budget := DefaultRunBudget()
	if err := budget.Validate(); err != nil {
		t.Fatalf("default budget invalid: %v", err)
	}
	if err := budget.Check(RunUsage{ModelTurns: budget.MaxModelTurns + 1}); !errors.Is(err, ErrRunBudgetExceeded) {
		t.Fatalf("budget error = %v", err)
	}
}
