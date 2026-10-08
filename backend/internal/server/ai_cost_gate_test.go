package server

import "testing"

func TestSetAICostGateAndOverBudget(t *testing.T) {
	t.Cleanup(func() { SetAICostGate(nil) })

	// No gate installed => never over budget on the dollar axis.
	if aiOverDollarBudget(123) {
		t.Fatal("no cost gate installed should never be over dollar budget")
	}

	// Install a gate that always reports over budget.
	SetAICostGate(alwaysOverGate{})
	if !aiOverDollarBudget(123) {
		t.Fatal("installed always-over gate should report over dollar budget")
	}
}

type alwaysOverGate struct{}

func (alwaysOverGate) OverBudget(_ uint) bool { return true }
