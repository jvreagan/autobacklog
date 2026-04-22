package claude

import (
	"fmt"
	"sync"
	"time"
)

// Budget tracks token/cost usage across invocations.
type Budget struct {
	mu            sync.Mutex
	maxTotal      float64
	spent         float64
	lastCost      float64
	invocations   int
	firstRecordAt time.Time
	now           func() time.Time
}

// NewBudget creates a budget tracker with a total spending limit.
// Negative maxTotal is treated as zero (#150).
func NewBudget(maxTotal float64) *Budget {
	if maxTotal < 0 {
		maxTotal = 0
	}
	return &Budget{maxTotal: maxTotal, now: time.Now}
}

// CanSpend checks if the given amount is within the remaining budget.
func (b *Budget) CanSpend(amount float64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent+amount <= b.maxTotal
}

// Record records spending from an invocation.
// Negative amounts are ignored (#150).
func (b *Budget) Record(amount float64) {
	if amount < 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.invocations == 0 {
		b.firstRecordAt = b.now()
	}
	b.spent += amount
	b.lastCost = amount
	b.invocations++
}

// LastCost returns the cost of the most recent invocation.
func (b *Budget) LastCost() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastCost
}

// Remaining returns the remaining budget.
func (b *Budget) Remaining() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.maxTotal - b.spent
}

// Spent returns the total amount spent.
func (b *Budget) Spent() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent
}

// Invocations returns the number of invocations recorded.
func (b *Budget) Invocations() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.invocations
}

// BurnRate returns the current spend rate in USD/hour.
// Returns 0 if fewer than 2 invocations have been recorded.
func (b *Budget) BurnRate() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.invocations < 2 {
		return 0
	}
	elapsed := b.now().Sub(b.firstRecordAt).Hours()
	if elapsed <= 0 {
		return 0
	}
	return b.spent / elapsed
}

// BurnRateExceeded reports whether the current burn rate exceeds maxRate.
// Returns false if maxRate <= 0 (disabled) or fewer than 2 invocations.
func (b *Budget) BurnRateExceeded(maxRate float64) bool {
	if maxRate <= 0 {
		return false
	}
	return b.BurnRate() > maxRate
}

// String returns a human-readable budget status.
// Handles singular/plural for "invocation(s)" (#201).
func (b *Budget) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	noun := "invocations"
	if b.invocations == 1 {
		noun = "invocation"
	}
	return fmt.Sprintf("$%.2f / $%.2f spent (%d %s)", b.spent, b.maxTotal, b.invocations, noun)
}
