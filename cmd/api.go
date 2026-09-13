package cmd

import (
	core "github.com/pskclub/mine-core/v2"
)

// APIRun serves HTTP only (APP_ROLE unset or "api").
func APIRun(app *core.App) {
	mods, err := Modules(app)
	if err != nil {
		app.Log().Error("modules", "err", err)
		return
	}

	serve(app, NewAPI(app, mods, defaultHTTPOptions()), nil, mods)
}

// NewAPI builds this deployment's HTTP server: the probes, and the modules'
// routes mounted onto it.
//
// It knows no module by name. That list is cmd.Modules, and what this adds is
// only what belongs to the *server* rather than to any feature — which is why
// the probes live here: the readiness probe has to be handed every module's own
// checks, and only the composition root holds them all. Registered from inside
// one module it would report fewer dependencies than the service has, and say
// nothing about being incomplete.
//
// Routes are mounted here rather than left to serve's core.RunModules because
// the tests call this function and never build a Runner (see testkit.Serve).
// Mounting the same server twice is a no-op, so serve is free to mount again.
//
// core.NewHTTPServer already applies request-id, structured request logging,
// panic recovery, CORS and the IError renderer, so a module only registers
// routes.
func NewAPI(app *core.App, mods *core.ModuleSet, opts *core.HTTPOptions) *core.Server {
	e := core.NewHTTPServer(app, opts)

	// /healthz and /readyz, from the framework rather than by hand: it holds
	// every pool that would be probed, so it can answer readiness properly —
	// each connection pinged in parallel, a critical one down answering 503 —
	// and mods.HealthChecks() adds what the framework cannot see by itself.
	//
	// They go on the server, not on an authenticated group: the orchestrator
	// calling them has no token.
	core.RegisterHealthRoutes(e, core.HealthOptions{Checks: mods.HealthChecks()})

	mods.MountHTTP(e)

	return e
}

// defaultHTTPOptions is what this deployment is configured with, as opposed to
// what the service is made of. Kept apart from NewAPI so the tests can stand up
// the same routes without inheriting production's CORS.
func defaultHTTPOptions() *core.HTTPOptions {
	return &core.HTTPOptions{
		AllowOrigins: []string{"*"},
	}
}
