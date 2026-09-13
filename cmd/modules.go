package cmd

import (
	"github.com/pskclub/mine-core-template/middlewares"
	"github.com/pskclub/mine-core-template/modules/auth"
	"github.com/pskclub/mine-core-template/modules/exchange"
	"github.com/pskclub/mine-core-template/modules/home"
	"github.com/pskclub/mine-core-template/modules/note"
	"github.com/pskclub/mine-core-template/modules/user"
	core "github.com/pskclub/mine-core/v2"
)

// Modules is the composition root: the one function that knows every module and
// how they fit together.
//
// Each module declares everything it attaches to the service in its own
// <name>.module.go — routes, jobs, cron, consumers, health checks — and this
// assembles them. What actually runs is then the *role's* decision rather than
// the module's: an api replica has no scheduler, so auth's Cron is never armed,
// out of this same list. One list, every role, no way for two of them to drift.
//
// Modules do not import each other. Each declares what it needs as a narrow
// interface of its own and this supplies it — which is what keeps the import
// graph a tree instead of a web, and is also why there is no auto-registration:
// a module registered by an init() takes no arguments and so could never be
// given usersFor.
//
// Exported because the tests stand up the real service through it (see
// testkit.Serve), so a module wired with a dependency left unsatisfied fails in
// a test rather than in production.
func Modules(app *core.App) (*core.ModuleSet, core.IError) {
	// Identity, wired once, before any route is registered.
	//
	// auth reads accounts through the user module's service rather than through
	// the users table — see modules/auth/service/auth.deps.go for why that is an
	// interface and not an import. The finished lookup then goes to the
	// middlewares package, which is what lets every module write
	// middlewares.AuthRequire(e) on a route without importing auth.
	usersFor := func(ctx core.IContext) auth.Users { return user.NewUserService(ctx) }
	middlewares.RegisterAuth(app, auth.ResolveToken(app, usersFor))

	return core.NewModules(
		home.New(),
		auth.New(usersFor),
		user.New(),
		note.New(),
		exchange.New(),
	)
}
