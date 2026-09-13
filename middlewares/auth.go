// Package middlewares holds the middleware a route reaches for by name.
//
// A route says what it needs and reads as one line:
//
//	e.GET("/users", c.Pagination, middlewares.AuthRequire(e))
//
// The point is that nothing has to be threaded through to get here. Passing the
// guard down as a parameter — every NewXxxHTTP taking one, every caller
// supplying it — made the answer to "what protects this route" three files away
// from the route.
//
// # Why registration
//
// Verifying a token means reading two tables that belong to two modules:
// access_tokens is auth's, users is the user module's. If this package imported
// them it would close a cycle, because their routes sit behind this middleware.
//
// So it imports neither. cmd.NewAPI — the one place that knows every
// module — hands it the finished lookup with RegisterAuth, and this package
// stores it against the App it belongs to. The cost is that the lookup is wired
// at startup rather than named in the type system; the arrangement that would
// avoid that is the parameter this replaces.
package middlewares

import (
	"context"
	"sync"

	"github.com/labstack/echo/v5"

	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/errmsgs"
)

// TokenResolver turns the SHA-256 digest of a token into the caller it names.
//
// The digest, not the token: core.HashedTokenVerifier hashes whatever the
// request carried before this is called, so a plaintext token never reaches the
// lookup and a leaked database yields nothing that resolves.
type TokenResolver func(ctx context.Context, tokenHash string) (*core.ContextUser, error)

// guards holds one *core.Auth per App.
//
// Keyed by App rather than kept in a single variable because a process can have
// more than one: every test builds its own, against its own database. A single
// global would work until the first t.Parallel(), and would then authenticate
// one test's requests against another test's data — a failure that reads like
// anything except what it is.
var guards sync.Map // *core.App -> *core.Auth

// RegisterAuth installs the token lookup for an App. cmd.NewAPI calls it
// once, before it registers any module.
func RegisterAuth(app *core.App, resolve TokenResolver) {
	guards.Store(app, core.NewAuth(core.HashedTokenVerifier(resolve)))
}

// AuthRequire rejects a request that does not carry a valid token.
//
//	e.POST("/notes", c.Create, middlewares.AuthRequire(e))
//
// It is written on each route rather than applied to a group. A group is safer —
// a route added later is protected by default — but it also means the only way
// to know whether a route is protected is to scroll up. Written out, each line
// answers that by itself, and every module has a test that names every route and
// asserts an anonymous caller is refused.
func AuthRequire(e *core.Server) echo.MiddlewareFunc {
	return authFor(e, (*core.Auth).Middleware)
}

// AuthOptional resolves a token when one is present and lets the request through
// when it is not, for endpoints that show more to a signed-in caller.
// c.GetUser() is nil for an anonymous one.
func AuthOptional(e *core.Server) echo.MiddlewareFunc {
	return authFor(e, (*core.Auth).Optional)
}

// AuthRole is AuthRequire plus a check on the caller's role.
//
//	e.DELETE("/users/:id", c.Delete, middlewares.AuthRole(e, consts.RoleAdmin))
//
// Only "who is calling" and "in what role". Whether that caller may touch *this
// row* is a business rule and belongs in the module's service, where it can see
// the row.
func AuthRole(e *core.Server, roles ...string) echo.MiddlewareFunc {
	return authFor(e, func(a *core.Auth) echo.MiddlewareFunc {
		return a.RequireRole(roles...)
	})
}

// authFor picks the middleware off the App's guard, once, at route registration.
//
// A server built without RegisterAuth gets a middleware that refuses every
// request instead of a nil dereference on the first one — the mistake is
// possible only in a hand-rolled server, and this is what makes it legible when
// it happens.
func authFor(e *core.Server, pick func(*core.Auth) echo.MiddlewareFunc) echo.MiddlewareFunc {
	stored, found := guards.Load(e.App())
	if !found {
		// At registration, so it is on stdout before the first request rather
		// than once per request afterwards. Nothing else will report this: the
		// server starts, the routes exist, and every call to them 500s.
		e.App().Log().Error("authentication is not configured",
			"hint", "call middlewares.RegisterAuth(app, ...) before registering routes; see cmd.NewAPI")

		return notConfigured
	}

	return pick(stored.(*core.Auth))
}

func notConfigured(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		return errmsgs.InternalServerError.
			WithMessage("authentication is not configured: middlewares.RegisterAuth was never called")
	}
}
