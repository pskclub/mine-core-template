package cmd

import (
	core "github.com/pskclub/mine-core/v2"
)

// AllRun serves HTTP *and* runs scheduled jobs in one process (APP_ROLE=all).
//
// Both share the one App, so there is one set of connection pools rather than
// two, and shutdown is ordered across both: requests drain, then jobs finish,
// then the pools close.
//
// It is also the same module list as the other two roles — this role is simply
// the one that has somewhere to put every extension point, so a module's routes
// and its cron both run here.
//
// ⚠️ Run only ONE replica in this role. The default scheduler queue is
// in-memory and per-process, so N replicas means every cron tick fires N times.
// To scale the API, run `api` replicas plus a single `worker` instead.
func AllRun(app *core.App) {
	mods, err := Modules(app)
	if err != nil {
		app.Log().Error("modules", "err", err)
		return
	}

	sc, err := newScheduler(app)
	if err != nil {
		app.Log().Error("scheduler", "err", err)
		return
	}

	serve(app, NewAPI(app, mods, defaultHTTPOptions()), sc, mods)
}
