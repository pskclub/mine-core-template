package cmd

import (
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/devtools"
)

// mountInspectors adds the read-only inspector at /_dev: capabilities and pool
// stats, routes and who serves them, config with secrets withheld, jobs and
// their runs, the modules tab, health.
//
// When it is on:
//
//	ENV=dev                   on, unprotected — a laptop
//	DEVTOOLS_PASSWORD set     on anywhere, behind a browser login
//	neither                   off, and the routes do not exist
//
// That gate is this project's, not the framework's. mine-core *refuses to
// mount* outside dev without a guard, which would stop a production boot; asking
// the same question first turns "refuse to start" into "do not mount", so
// turning the panel on in staging is a deployment change and not a code change.
//
// Writing — triggering a job, cancelling a run — stays off outside dev even when
// the panel is open behind a password. Reading what a process is doing and
// re-running last night's billing job are not the same permission.
//
// Called from serve, not NewAPI: the panel is a deployment concern, so the tests
// that stand up the service through NewAPI never get it. /_docs is the other
// panel, and it is mounted in NewAPI instead because it is public by decision
// (see there).
func mountInspectors(app *core.App, e *core.Server, sc *core.Scheduler, mods *core.ModuleSet) {
	if e == nil {
		return // a worker has nothing to mount on
	}
	env := app.ENV()
	if !env.IsDev() && env.String(devtools.EnvBasicAuthPassword) == "" {
		return
	}

	opts := devtools.Options{
		Modules:    mods,
		AllowWrite: env.IsDev(),
		// BasicAuth is left nil on purpose: devtools reads DEVTOOLS_USER and
		// DEVTOOLS_PASSWORD from the environment itself, which is what makes
		// the gate above true without this file repeating it.
	}
	if sc != nil {
		opts.Runner = sc.Runner()
	}

	if err := devtools.Mount(e, opts); err != nil {
		// Not fatal: a service that cannot show its inspector is still a
		// service. The line is the only trace of a panel somebody expected.
		app.Log().Error("devtools", "err", err)
	}
}
