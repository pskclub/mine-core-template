// Package repo holds query helpers that belong to no single table.
//
// Repositories themselves do *not* live here. Each module opens its own, keeps
// it unexported, and answers other modules through its service — so a table has
// exactly one package that can write to it. A shared repo package would undo
// that: it is the shortcut through which, a year in, every module ends up
// querying every table and no column can be changed.
//
// What is left here is the model-agnostic part: helpers that shape a query
// without knowing which table it runs against. Anything that names a model
// belongs to that model's module.
package repo

import (
	core "github.com/pskclub/mine-core/v2"
)

// DefaultOrder supplies an ordering for a list request that did not ask for one.
//
// That is the *whole* job: Pagination already applies opts.OrderBy itself, so
// adding an Order() to the chain as well emits the clause twice —
// "ORDER BY created_at desc,created_at desc".
//
// The result is a copy when a default is applied, matching the repository's own
// copy-on-write style: a shared PageOptions is never mutated behind the caller.
// Passing nil is fine and yields a usable options value.
func DefaultOrder(opts *core.PageOptions, order ...string) *core.PageOptions {
	if opts == nil {
		return &core.PageOptions{OrderBy: order}
	}
	if len(opts.OrderBy) > 0 {
		return opts
	}

	cp := *opts
	cp.OrderBy = order
	return &cp
}
