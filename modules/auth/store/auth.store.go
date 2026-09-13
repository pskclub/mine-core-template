// Package store is how the auth module reaches its own table, and the only
// package that does.
//
// access_tokens belongs to this module. Nothing outside modules/auth may import
// this package — arch enforces it — so a module that needs something
// from the table asks auth for it through a method, and auth decides what it is
// willing to answer.
//
// The functions here are exported only because the module's own service package
// sits in a different directory and has to reach them. That is what splitting a
// module into directories costs: the boundary is checked in CI instead of by the
// compiler.
package store

import (
	"github.com/pskclub/mine-core-template/models"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/repository"
)

// AccessToken opens the access_tokens repository on ctx. The context (deadline,
// cancellation, trace) is bound once here, so no query method takes one again.
func AccessToken(ctx core.IContext) *repository.Repo[models.AccessToken] {
	return repository.New[models.AccessToken](ctx)
}
