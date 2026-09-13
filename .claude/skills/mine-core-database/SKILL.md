---
name: mine-core-database
description: Work with the database in this mine-core service — adding or changing a table (prisma migration + Go model + testkit registration), writing a module store on repository.Repo[M], scopes, pagination, transactions, soft vs hard delete, and the two GORM traps. Use whenever a schema, model, store or query changes.
---

# Schema, models and stores

## Adding or changing a table

**Nothing in Go migrates the database.** Prisma owns the schema; Go owns
everything else. The order matters:

1. Edit `prisma/schema/<module>.prisma` — **one file per module**, matching the
   Go layout ([notes.prisma](../../../prisma/schema/notes.prisma)).
2. `make migrate-dev` — authors a migration on the host against the postgres
   published on localhost (it prompts for a name and needs a shadow database).
3. Add the struct in [models/](../../../models/).
4. **Register it in `allModels()` in [testkit/testkit.go](../../../testkit/testkit.go)** —
   the sqlite suite builds its tables from that list. Forgetting this is a
   "no such table" that looks like a code bug.
5. `make generate` if `prisma/seed.ts` touches it.
6. `make migrate` applies committed migrations in a container — the same step CI runs.

**Do not declare prisma relations across module boundaries.** A relation invites
a join, and a join across two modules' tables is exactly the coupling this layout
prevents. Store `user_id` as a plain column and resolve names through the owning
module's service. Index for the real access pattern
(`@@index([user_id, created_at])`, not `user_id` alone).

## Models

`models/` is the shared kernel: structs and column tags, **no behaviour**, and it
imports nothing from this repository (arch enforces both).

```go
type Note struct {
    BaseModel
    UserID string `json:"user_id" gorm:"column:user_id"`
    Title  string `json:"title" gorm:"column:title"`
    Pinned bool   `json:"pinned" gorm:"column:pinned"`
}

func (Note) TableName() string { return "notes" }
```

`models.BaseModel` gives `ID`/`CreatedAt`/`UpdatedAt`/`DeletedAt` (soft delete);
`BaseModelHardDelete` omits `DeletedAt`. Construct with
`models.NewBaseModel()` — it fills a UUID and both timestamps.

No GORM associations to another module's table: `Preload` would let any package
holding the struct read rows it does not own.

## The store

One package per module, the **only** one that queries that module's tables:

```go
// modules/note/store/note.store.go
func Note(ctx core.IContext) *repository.Repo[models.Note] {
    return repository.New[models.Note](ctx)
}

// Scopes are how a rule that must never be forgotten is written once.
func OwnedBy(ownerID string) func(*gorm.DB) *gorm.DB {
    return func(db *gorm.DB) *gorm.DB { return db.Where("user_id = ?", ownerID) }
}

func Search(q string) func(*gorm.DB) *gorm.DB {
    return func(db *gorm.DB) *gorm.DB {
        q = strings.TrimSpace(q)
        if q == "" {
            return db                    // always safe to pass
        }
        // LOWER(...) LIKE, not ILIKE — `make test` runs on sqlite
        return db.Where("LOWER(title) LIKE ?", "%"+strings.ToLower(q)+"%")
    }
}
```

Stores are **silent by design** — the framework's GORM logger already reports
failures, slow statements, and every statement at `LOG_LEVEL=debug`. What a
query *meant* is logged one layer up, in the service.

Query helpers that name no model go in [repo/](../../../repo/). Repositories
themselves never do — a shared repo package is the shortcut through which every
module ends up querying every table.

## `repository.Repo[M]`

Chainable and copy-on-write, so a shared value is never mutated behind a caller.

```
read    FindOne FindAll Take Last Count Exists Pluck Scan Raw FindInBatches
write   Create CreateInBatches Save Update Updates Delete HardDelete Exec
        FindOneOrCreate FindOneOrInit
shape   Where Or Not Select Omit Order Limit Offset Group Having Distinct Table
        Joins InnerJoins Preload Scopes Unscoped Clauses Attrs Assign
page    Pagination(opts) -> *core.Page[M]
tx      Transaction(func(tx *gorm.DB) error)
escape  DB() *gorm.DB
```

Every terminal method returns `core.IError` already typed — a missing row is
`NotFound`, a driver failure is `DBError` — so the service wraps with
`s.ctx.NewError(err, err)` and keeps that status.

## The two traps

**1. `Updates` with a struct skips zero values.** `pinned` going `true` → `false`
is silently dropped and the row comes back still pinned. Any column that can
legitimately hold `false`, `0` or `""` must be written with a map:

```go
store.Note(s.ctx).Scopes(store.OwnedBy(ownerID)).Where("id = ?", id).
    Updates(map[string]any{"title": input.Title, "pinned": input.Pinned})
```

**2. `repo.DefaultOrder(opts, ...)`, not `.Order()`.** `Pagination` already
applies `opts.OrderBy`; adding `Order()` emits the clause twice.

```go
opts := repo.DefaultOrder(pageOptions, "pinned DESC", "created_at DESC")
page, err := store.Note(s.ctx).Scopes(store.OwnedBy(ownerID), store.Search(opts.Q)).Pagination(opts)
```

## Ownership

Thread it as an explicit first argument (`ownerID string`) and apply it as a
**scope on every statement** — including the write that follows a read, because a
filter present on only one of two statements is the shape most ownership bugs
take. Never fetch and compare afterwards. "Not yours" answers **404**.

## Soft vs hard delete

`Delete` sets `deleted_at` and every later query hides the row. `HardDelete`
removes it and implies `Unscoped`, so it also sweeps rows a soft delete already
hid — which is what a retention job wants
([auth.job.go](../../../modules/auth/service/auth.job.go)). Use `Unscoped()` to
read soft-deleted rows deliberately.

## Transactions

```go
err := store.Note(ctx).Transaction(func(tx *gorm.DB) error {
    // every statement inside must use tx
    return nil                     // non-nil rolls back
})
```

Keep them inside one module — a transaction spanning two modules' tables is a
sign the boundary is wrong. Details and the nested/savepoint cases:
`$CORE/docs/database-transactions.md`.

## Before pushing

`make test` runs sqlite in memory. **`make test-integration` runs the real prisma
schema on postgres** — sqlite has no partial indexes, no UUID type and no
`ILIKE`, so it cannot reject a schema postgres would. Run it after any schema
change; it needs nothing installed, because [cmd/testpg](../../../cmd/testpg/)
starts a real postgres 17 for the run and throws it away.

Long form: `$CORE/docs/repository*.md`, `database-pagination.md`,
`database-transactions.md`.
