package app_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/app"
)

func TestBudgetReserveBytes(t *testing.T) {
	t.Parallel()

	b := app.NewBudget(app.BudgetLimits{Bytes: 10, Items: 0})

	if got := b.ReserveBytes(4); got != 4 {
		t.Errorf("ReserveBytes(4) = %d, want 4", got)
	}

	if got := b.ReserveBytes(100); got != 6 {
		t.Errorf("ReserveBytes(100) = %d, want the 6 bytes left", got)
	}

	if got := b.ReserveBytes(1); got != 0 {
		t.Errorf("ReserveBytes(1) on an empty budget = %d, want 0", got)
	}

	b.Bytes.Add(3)

	if got := b.ReserveBytes(100); got != 3 {
		t.Errorf("ReserveBytes(100) after RefundBytes(3) = %d, want 3", got)
	}
}

func TestBudgetSpendItems(t *testing.T) {
	t.Parallel()

	b := app.NewBudget(app.BudgetLimits{Bytes: 0, Items: 3})

	if !b.SpendItems(2) {
		t.Error("SpendItems(2) = false, want true")
	}

	if b.SpendItems(2) {
		t.Error("SpendItems(2) with 1 left = true, want false")
	}

	if !b.SpendItems(1) {
		t.Error("SpendItems(1) with 1 left = false, want true, since a failed spend takes nothing")
	}
}

func TestBudgetIsSharedAcrossGoroutines(t *testing.T) {
	t.Parallel()

	const limit = 50

	b := app.NewBudget(app.BudgetLimits{Bytes: limit, Items: limit})

	var (
		wg            sync.WaitGroup
		spentItems    atomic.Int64
		reservedBytes atomic.Int64
	)

	for range 4 * limit {
		wg.Go(func() {
			if b.SpendItems(1) {
				spentItems.Add(1)
			}

			reservedBytes.Add(b.ReserveBytes(1))
		})
	}

	wg.Wait()

	if got := spentItems.Load(); got != limit {
		t.Errorf("goroutines spent %d items in total, want exactly %d", got, limit)
	}

	if got := reservedBytes.Load(); got != limit {
		t.Errorf("goroutines reserved %d bytes in total, want exactly %d", got, limit)
	}
}
