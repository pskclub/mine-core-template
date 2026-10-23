package cmd

import (
	"github.com/labstack/echo/v5"
	"github.com/pskclub/mine-core-template/apispec"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/apidocs"
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

	// No framing, no MIME sniffing, a conservative referrer policy, HSTS on
	// TLS. The framework leaves this out of its default stack because a server
	// that renders HTML wants its own policy; this one is a JSON API, so the
	// defaults are right as they are.
	e.Use(core.SecureHeaders())

	// /healthz and /readyz, from the framework rather than by hand: it holds
	// every pool that would be probed, so it can answer readiness properly —
	// each connection pinged in parallel, a critical one down answering 503 —
	// and mods.HealthChecks() adds what the framework cannot see by itself.
	//
	// They go on the server, not on an authenticated group: the orchestrator
	// calling them has no token.
	core.RegisterHealthRoutes(e, core.HealthOptions{Checks: mods.HealthChecks()})

	// /_docs is public in every environment, not only in dev. apidocs refuses
	// to mount unguarded outside dev, so it is handed a guard that lets every
	// request through — the decision to publish the reference is made here, on
	// purpose, rather than by the framework's default.
	if err := apidocs.Mount(e, apidocs.Options{
		Spec: apispec.Spec,
		Auth: []echo.MiddlewareFunc{public},
	}); err != nil {
		app.Log().Error("apidocs", "err", err)
	}

	mods.MountHTTP(e)

	return e
}

// public is a guard that admits every request.
func public(next echo.HandlerFunc) echo.HandlerFunc { return next }

// defaultHTTPOptions is what this deployment is configured with, as opposed to
// what the service is made of. Kept apart from NewAPI so the tests can stand up
// the same routes without inheriting production's CORS.
func defaultHTTPOptions() *core.HTTPOptions {
	return &core.HTTPOptions{
		AllowOrigins: []string{"*"},
		// The probes answer every few seconds and are most of a quiet
		// service's log. A probe that *fails* is still logged — that line is
		// the one that explains why the pod was pulled. LOG_REQUEST_SKIP_PATHS
		// in the environment would do the same; this is the deployment's
		// default so nobody has to remember to set it.
		SkipRequestLogPaths: []string{"/healthz", "/readyz", "/_dev/*"},
	}
}
