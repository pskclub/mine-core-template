package cmd

import (
	"os"
	"time"

	core "github.com/pskclub/mine-core/v2"
)

// shutdownTimeout bounds the whole drain. Raise it above your longest expected
// request or job — and keep it *below* the orchestrator's grace period
// (Kubernetes' terminationGracePeriodSeconds), or the process is killed
// mid-drain and the ordering below never actually happens.
const shutdownTimeout = 15 * time.Second

// serve starts whichever parts this role has — either may be nil — and blocks
// until SIGINT/SIGTERM, then stops them in the only order that is safe: stop
// producing work, drain what is already running, close the pools last.
//
// core.Runner owns that sequence. This used to be written out here, along with a
// comment explaining why core.StartHTTPServer could not be used: the scheduler
// has to stop *between* the drain and the pool close, and a single call had no
// seam for it. The Runner is that seam.
func serve(app *core.App, e *core.Server, sc *core.Scheduler, mods *core.ModuleSet) {
	// RunModules attaches every module to whichever pieces this role has, before
	// anything starts: cron only when there is a scheduler, routes only when
	// there is a server. The boot line names what it had nowhere to put
	// (skipped=[cron] in an api replica), so a role that was meant to arm those
	// schedules says so instead of leaving it to be worked out from a job that
	// never ran.
	opts := []core.RunnerOption{
		core.WithDrainTimeout(shutdownTimeout),
		core.RunModules(mods),
	}

	if e != nil {
		opts = append(opts, core.RunHTTP(e))
	}
	if sc != nil {
		// both: the scheduler stops first so no tick queues a run during the
		// drain, then the job runner is given the drain to finish what it holds
		opts = append(opts, core.RunScheduler(sc), core.RunJobs(sc.Runner()))
	}

	if err := core.NewRunner(app, opts...).Run(); err != nil {
		// a non-zero exit, so a service that could not bind or could not start
		// is reported as failed rather than as a clean stop the orchestrator
		// should leave alone
		app.Log().Error("service stopped", "err", err)
		os.Exit(1)
	}
}
