// Package store is how the note module reaches its own table, and the only
// package that does.
//
// notes belongs to this module. Nothing outside modules/note may import this
// package — arch enforces it — so the ownership scope below cannot be
// forgotten by someone writing a query somewhere else.
//
// Nothing here logs, on purpose. The framework already has a GORM logger:
// failures and slow statements always, and every statement at LOG_LEVEL=debug.
// A line of our own would repeat what it says, without the SQL or the timing
// that make it useful. What a query *meant* is logged one layer up, in service.
package store

import (
	"strings"

	"github.com/pskclub/mine-core-template/models"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/repository"
	"gorm.io/gorm"
)

// Note opens the notes repository on ctx.
func Note(ctx core.IContext) *repository.Repo[models.Note] {
	return repository.New[models.Note](ctx)
}

// OwnedBy is the scope every read and write in this module applies.
//
// Ownership is enforced as a WHERE clause rather than as a check after loading,
// which is what makes "not yours" and "not there" the same answer. Fetching the
// row first and comparing user_id afterwards would work, but a caller could then
// tell an id that exists from one that does not by the shape of the failure.
func OwnedBy(ownerID string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("user_id = ?", ownerID)
	}
}

// Search matches a list request's free-text `q` against the title.
//
// An empty q returns the query untouched, so it is always safe to pass.
// LOWER(...) LIKE rather than ILIKE keeps this working on sqlite too, which is
// what `make test` runs against.
func Search(q string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		q = strings.TrimSpace(q)
		if q == "" {
			return db
		}

		return db.Where("LOWER(title) LIKE ?", "%"+strings.ToLower(q)+"%")
	}
}
