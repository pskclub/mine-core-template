package service

import (
	"github.com/pskclub/mine-core-template/models"
	core "github.com/pskclub/mine-core/v2"
)

// Users is the part of the user module that auth needs, declared here rather
// than imported from there.
//
// This is the rule the whole layout rests on: a module never reaches into
// another module's tables. auth owns access_tokens; users belongs to the user
// module, so every read and write of it goes through that module's service.
//
// Why an interface instead of just importing it: user's routes sit behind auth's
// guard, so user already depends on auth. An import back would close the loop and
// stop compiling. Naming the dependency in the consumer — the shape auth needs,
// not the shape user happens to have — keeps the arrow pointing one way, and
// cmd.NewAPI supplies the implementation.
//
// modules/auth re-exports both of these as aliases, so httpapi never has to
// import this package. Nothing outside modules/auth does.
//
// Keep it narrow. Every method added here is another thing auth can do to a
// table it does not own.
type Users interface {
	// Find returns the account with this id, or a not-found error.
	Find(id string) (*models.User, core.IError)

	// FindByEmail is the sign-in lookup. It must return the row including the
	// password hash — Login has nothing to compare against otherwise.
	FindByEmail(email string) (*models.User, core.IError)

	// CreateAccount registers an account. The hash is computed by auth, which
	// owns the credential policy; the user module only stores what it is given
	// and must never be handed a plaintext password.
	CreateAccount(email, fullName, passwordHash string) (*models.User, core.IError)
}

// UsersFor opens the user service on a context.
//
// A factory rather than a value because every service in this project is bound
// to one context — a request's deadline, its transaction, its trace. There is no
// long-lived user service to hold.
type UsersFor func(core.IContext) Users
