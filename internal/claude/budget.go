package claude

import (
	"fmt"
	"sync"
)

// Budget tracks token/cost usage across invocations.
type Budget struct {
	mu          sync.Mutex
	maxTotal    float64
	spent       float64
	invocations int
}

// NewBudget creates a budget tracker with a total spending limit.
func NewBudget(maxTotal float64) *Budget {
	return &Budget{maxTotal: maxTotal}
}

// CanSpend checks if the given amount is within the remaining budget.
func (b *Budget) CanSpend(amount float64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent+amount <= b.maxTotal
}

// Record records spending from an invocation.
func (b *Budget) Record(amount float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.spent += amount
	b.invocations++
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

// String returns a human-readable budget status.
func (b *Budget) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return fmt.Sprintf("$%.2f / $%.2f spent (%d invocations)", b.spent, b.maxTotal, b.invocations)
}
