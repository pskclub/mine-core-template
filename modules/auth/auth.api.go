package auth

import (
	"github.com/pskclub/mine-core-template/middlewares"
	"github.com/pskclub/mine-core-template/modules/auth/service"
	core "github.com/pskclub/mine-core/v2"
)

// This file is the module's public surface: everything another package may
// reach, gathered in one place and forwarding to the implementation.
//
// The rule it exists to keep true is that a module is entered only through its
// top-level package. cmd.Modules imports modules/auth and never
// modules/auth/service, so the import graph stays one arrow per module rather
// than one per layer — and moving a type between handler/, service/ and store/
// stays an edit inside this directory. arch fails the build on an
// import that goes around it.
//
// Aliases, not new types: service.Users and auth.Users are the same type, so an
// implementation satisfies both and nothing has to be converted.

// Users is the part of the user module that auth needs. See
// service/auth.deps.go for why it is an interface declared here rather than an
// import of the user module.
type Users = service.Users

// UsersFor opens the user service on a context.
type UsersFor = service.UsersFor

// IAuthService is the module's business logic, for a caller that has a context
// already — a job, or a test.
type IAuthService = service.IAuthService

// NewAuthService builds the service on a context, given the user service it
// reads accounts through.
func NewAuthService(ctx core.IContext, users Users) IAuthService {
	return service.NewAuthService(ctx, users)
}

// ResolveToken is how a token becomes a caller. cmd.Modules hands it to
// middlewares.RegisterAuth, which is what makes middlewares.AuthRequire work
// without the middlewares package importing this module — it must not, since
// this module's own routes sit behind that middleware.
//
// It takes an App rather than a context because the middleware runs before any
// handler: the request's context arrives per call and is bound inside.
func ResolveToken(app *core.App, usersFor UsersFor) middlewares.TokenResolver {
	return service.ResolveToken(app, usersFor)
}
