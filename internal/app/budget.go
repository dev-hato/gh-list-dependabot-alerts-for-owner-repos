package app

import "sync/atomic"

const (
	// MaxTotalBytes caps how much response body the whole process decodes across all requests.
	// Without it, MaxPages pages of MaxPageBytes each, times every repository, would be far more than memory can hold.
	MaxTotalBytes = 1 << 30
	// MaxTotalItems caps how many decoded items the whole process keeps across all requests.
	// Tiny JSON items like "{}" decode into much larger structs, so the byte cap alone doesn't bound memory.
	MaxTotalItems = 100000
)

// budget is the process-wide allowance every fetch spends from,
// so parallel per-repository fetches share one cap instead of each getting their own.
var budget = NewBudget(BudgetLimits{Bytes: MaxTotalBytes, Items: MaxTotalItems})

// BudgetLimits is the starting allowance for a Budget.
type BudgetLimits struct {
	Bytes int64
	Items int64
}

// Budget is a byte and item allowance for decoded responses.
// It is safe for concurrent use.
type Budget struct {
	Bytes atomic.Int64
	items atomic.Int64
}

// NewBudget returns a Budget starting at limits.
func NewBudget(limits BudgetLimits) *Budget {
	b := &Budget{}
	b.Bytes.Store(limits.Bytes)
	b.items.Store(limits.Items)
	return b
}

// ReserveBytes takes up to n bytes from the budget and returns how many it took.
// Reserving before reading keeps concurrent readers from spending the same bytes twice.
// Give back whatever goes unread with RefundBytes.
func (b *Budget) ReserveBytes(n int64) int64 {
	for {
		remaining := b.Bytes.Load()
		granted := min(n, remaining)

		if b.Bytes.CompareAndSwap(remaining, remaining-granted) {
			return granted
		}
	}
}

// SpendItems takes n items from the budget.
// It reports false, taking nothing, when fewer than n are left.
func (b *Budget) SpendItems(n int64) bool {
	for {
		remaining := b.items.Load()

		if remaining < n {
			return false
		}

		if b.items.CompareAndSwap(remaining, remaining-n) {
			return true
		}
	}
}
