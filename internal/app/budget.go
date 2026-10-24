package app

import (
	"io"
	"sync/atomic"

	"github.com/cockroachdb/errors"
)

const (
	// MaxTotalBytes caps how much response body the whole process decodes across all requests.
	// Without it, MaxPages pages of MaxPageBytes each, times every repository, would be far more than memory can hold.
	MaxTotalBytes = 1 << 30
	// MaxTotalItems caps how many decoded items the whole process keeps across all requests.
	// Tiny JSON items like "{}" decode into much larger structs, so the byte cap alone doesn't bound memory.
	MaxTotalItems = 100000
)

// ErrByteBudgetExceeded is what reading response bodies past the budget's byte limit fails with.
var ErrByteBudgetExceeded = errors.New("Response bodies exceed the total byte limit")

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
	bytes atomic.Int64
	items atomic.Int64
}

// NewBudget returns a Budget starting at limits.
func NewBudget(limits BudgetLimits) *Budget {
	b := &Budget{}
	b.bytes.Store(limits.Bytes)
	b.items.Store(limits.Items)
	return b
}

// SpendBytes takes n bytes from the budget.
// It reports false, taking nothing, when fewer than n are left.
func (b *Budget) SpendBytes(n int64) bool {
	return spend(&b.bytes, n)
}

// SpendItems takes n items from the budget.
// It reports false, taking nothing, when fewer than n are left.
func (b *Budget) SpendItems(n int64) bool {
	return spend(&b.items, n)
}

func spend(remaining *atomic.Int64, n int64) bool {
	for {
		left := remaining.Load()

		if left < n {
			return false
		}

		if remaining.CompareAndSwap(left, left-n) {
			return true
		}
	}
}

// BudgetReader spends every byte read from R from Budget,
// and fails with ErrByteBudgetExceeded once Budget has too few bytes left.
// It spends bytes only after they're read, never ahead of time,
// so a slow response holds nothing back from concurrent readers while it waits for data.
type BudgetReader struct {
	R      io.Reader
	Budget *Budget
}

func (r BudgetReader) Read(p []byte) (int, error) {
	n, err := r.R.Read(p)

	if !r.Budget.SpendBytes(int64(n)) {
		return 0, errors.WithStack(ErrByteBudgetExceeded)
	}

	// io.EOF must reach the caller unwrapped, since readers compare against it with ==.
	if err != nil && !errors.Is(err, io.EOF) {
		return n, errors.Wrap(err, "Failed to Read")
	}

	return n, err
}
