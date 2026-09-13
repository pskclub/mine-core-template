package repo

import (
	"testing"

	core "github.com/pskclub/mine-core/v2"
)

func TestDefaultOrder(t *testing.T) {
	t.Run("fills in the default when the request asked for no order", func(t *testing.T) {
		got := DefaultOrder(&core.PageOptions{Limit: 10}, "created_at DESC")

		if len(got.OrderBy) != 1 || got.OrderBy[0] != "created_at DESC" {
			t.Fatalf("OrderBy = %v, want [created_at DESC]", got.OrderBy)
		}
		if got.Limit != 10 {
			t.Errorf("Limit = %d, want the original 10", got.Limit)
		}
	})

	t.Run("keeps the request's own order", func(t *testing.T) {
		got := DefaultOrder(&core.PageOptions{OrderBy: []string{"email asc"}}, "created_at DESC")

		if len(got.OrderBy) != 1 || got.OrderBy[0] != "email asc" {
			t.Fatalf("OrderBy = %v, want the caller's [email asc]", got.OrderBy)
		}
	})

	t.Run("does not mutate the options it was given", func(t *testing.T) {
		opts := &core.PageOptions{Limit: 10}
		DefaultOrder(opts, "created_at DESC")

		if len(opts.OrderBy) != 0 {
			t.Errorf("input was mutated: OrderBy = %v, want empty", opts.OrderBy)
		}
	})

	t.Run("nil is usable", func(t *testing.T) {
		got := DefaultOrder(nil, "created_at DESC")

		if got == nil {
			t.Fatal("returned nil; callers dereference this")
		}
		if len(got.OrderBy) != 1 {
			t.Errorf("OrderBy = %v, want the default", got.OrderBy)
		}
	})
}
