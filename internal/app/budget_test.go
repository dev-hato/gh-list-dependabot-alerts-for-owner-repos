package app_test

import (
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/app"
)

func TestBudgetSpendBytes(t *testing.T) {
	t.Parallel()

	b := app.NewBudget(app.BudgetLimits{Bytes: 10, Items: 0})

	if !b.SpendBytes(4) {
		t.Error("SpendBytes(4) = false, want true")
	}

	if b.SpendBytes(7) {
		t.Error("SpendBytes(7) with 6 left = true, want false")
	}

	if !b.SpendBytes(6) {
		t.Error("SpendBytes(6) with 6 left = false, want true, since a failed spend takes nothing")
	}
}

func TestBudgetReader(t *testing.T) {
	t.Parallel()

	t.Run("a body within the budget reads in full", func(t *testing.T) {
		t.Parallel()

		b := app.NewBudget(app.BudgetLimits{Bytes: 5, Items: 0})

		got, err := io.ReadAll(app.BudgetReader{R: strings.NewReader("hello"), Budget: b})
		if err != nil || string(got) != "hello" {
			t.Errorf("ReadAll() = %q, %v, want %q, nil", got, err, "hello")
		}
	})

	t.Run("a body past the budget is an error", func(t *testing.T) {
		t.Parallel()

		b := app.NewBudget(app.BudgetLimits{Bytes: 4, Items: 0})

		_, err := io.ReadAll(app.BudgetReader{R: strings.NewReader("hello"), Budget: b})
		if !errors.Is(err, app.ErrByteBudgetExceeded) {
			t.Errorf("ReadAll() error = %v, want %v", err, app.ErrByteBudgetExceeded)
		}
	})
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
		wg         sync.WaitGroup
		spentItems atomic.Int64
		spentBytes atomic.Int64
	)

	for range 4 * limit {
		wg.Go(func() {
			if b.SpendItems(1) {
				spentItems.Add(1)
			}

			if b.SpendBytes(1) {
				spentBytes.Add(1)
			}
		})
	}

	wg.Wait()

	if got := spentItems.Load(); got != limit {
		t.Errorf("goroutines spent %d items in total, want exactly %d", got, limit)
	}

	if got := spentBytes.Load(); got != limit {
		t.Errorf("goroutines spent %d bytes in total, want exactly %d", got, limit)
	}
}
