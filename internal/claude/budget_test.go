package claude

import (
	"strings"
	"testing"
)

func TestBudget_NewBudget(t *testing.T) {
	b := NewBudget(100.0)
	if b.Remaining() != 100.0 {
		t.Errorf("Remaining() = %f, want 100.0", b.Remaining())
	}
	if b.Spent() != 0 {
		t.Errorf("Spent() = %f, want 0", b.Spent())
	}
	if b.Invocations() != 0 {
		t.Errorf("Invocations() = %d, want 0", b.Invocations())
	}
}

func TestBudget_CanSpend(t *testing.T) {
	b := NewBudget(10.0)

	if !b.CanSpend(5.0) {
		t.Error("should be able to spend 5 of 10")
	}
	if !b.CanSpend(10.0) {
		t.Error("should be able to spend exactly 10 of 10")
	}
	if b.CanSpend(10.01) {
		t.Error("should not be able to spend 10.01 of 10")
	}
}

func TestBudget_Record(t *testing.T) {
	b := NewBudget(100.0)

	b.Record(25.0)
	if b.Spent() != 25.0 {
		t.Errorf("Spent() = %f, want 25.0", b.Spent())
	}
	if b.Remaining() != 75.0 {
		t.Errorf("Remaining() = %f, want 75.0", b.Remaining())
	}
	if b.Invocations() != 1 {
		t.Errorf("Invocations() = %d, want 1", b.Invocations())
	}

	b.Record(30.0)
	if b.Spent() != 55.0 {
		t.Errorf("Spent() = %f, want 55.0", b.Spent())
	}
	if b.Invocations() != 2 {
		t.Errorf("Invocations() = %d, want 2", b.Invocations())
	}
}

func TestBudget_CanSpendAfterRecording(t *testing.T) {
	b := NewBudget(10.0)
	b.Record(8.0)

	if !b.CanSpend(2.0) {
		t.Error("should be able to spend remaining 2.0")
	}
	if b.CanSpend(3.0) {
		t.Error("should not be able to spend 3.0 with only 2.0 remaining")
	}
}

func TestBudget_String(t *testing.T) {
	b := NewBudget(100.0)
	b.Record(25.50)

	s := b.String()
	if !strings.Contains(s, "25.50") {
		t.Errorf("String() = %q, should contain spent amount", s)
	}
	if !strings.Contains(s, "100.00") {
		t.Errorf("String() = %q, should contain total budget", s)
	}
	if !strings.Contains(s, "1 invocation") {
		t.Errorf("String() = %q, should contain invocation count", s)
	}
}

func TestBudget_LastCost(t *testing.T) {
	b := NewBudget(100.0)

	if b.LastCost() != 0 {
		t.Errorf("LastCost() = %f, want 0 before any recording", b.LastCost())
	}

	b.Record(5.0)
	if b.LastCost() != 5.0 {
		t.Errorf("LastCost() = %f, want 5.0", b.LastCost())
	}

	b.Record(3.0)
	if b.LastCost() != 3.0 {
		t.Errorf("LastCost() = %f, want 3.0 (should be most recent)", b.LastCost())
	}

	// Negative amounts are ignored, LastCost should not change
	b.Record(-1.0)
	if b.LastCost() != 3.0 {
		t.Errorf("LastCost() = %f, want 3.0 (negative should be ignored)", b.LastCost())
	}
}
