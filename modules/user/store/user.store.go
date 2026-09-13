// Package store is how the user module reaches its own table, and the only
// package that does.
//
// users belongs to this module. Nothing outside modules/user may import this
// package — arch enforces it — so another module that needs an account,
// auth to sign someone in, calls IUserService and gets only what this module
// chose to offer. That is the rule that keeps a column changeable a year from
// now, when thirty modules exist and a shared repo package would have let all of
// them query this table directly.
package store

import (
	"strings"

	"github.com/pskclub/mine-core-template/models"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/repository"
	"gorm.io/gorm"
)

// User opens the users repository on ctx. The context (deadline, cancellation,
// trace) is bound once here, so no query method takes one again.
func User(ctx core.IContext) *repository.Repo[models.User] {
	return repository.New[models.User](ctx)
}

// Search matches a list request's free-text `q` against the fields worth
// searching. PageOptions carries Q but nothing applies it — naming the scope
// here is what keeps every caller searching the same fields.
//
// An empty q returns the query untouched, so it is always safe to pass.
// LOWER(...) LIKE rather than ILIKE keeps this working on any driver.
func Search(q string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		q = strings.TrimSpace(q)
		if q == "" {
			return db
		}

		like := "%" + strings.ToLower(q) + "%"
		return db.Where("LOWER(full_name) LIKE ?", like)
	}
}
