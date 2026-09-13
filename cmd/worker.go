package cmd

import (
	"time"

	core "github.com/pskclub/mine-core/v2"
)

// WorkerRun runs scheduled jobs only (APP_ROLE=worker). Jobs get the same
// capabilities as HTTP handlers because they run on the same App.
//
// It assembles the same module list as the API role: a module's Cron is armed
// here and its Routes have nowhere to go, which serve's boot line names
// (skipped=[http]) rather than leaving it to be inferred.
func WorkerRun(app *core.App) {
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

	serve(app, nil, sc, mods)
}

// newScheduler builds the scheduler and arms the work that belongs to the
// *process* rather than to any feature.
//
// It knows no module. A module owns its jobs the same way it owns its tables
// and its routes — the sweep that deletes from access_tokens is an auth
// decision, so it is auth.Module.Cron — and core.RunModules arms every one of
// them on this scheduler. Adding a job touches only its module; nothing here.
func newScheduler(app *core.App) (*core.Scheduler, core.IError) {
	// An explicit job runner rather than the one NewScheduler would build for
	// itself. When the scheduler owns it, the scheduler also stops it — on its
	// own built-in timeout, during the "stop producing work" step. Handing it
	// over means core.Runner starts and drains it instead, with this service's
	// shutdownTimeout, at the point in the sequence where draining belongs.
	sc, err := core.NewScheduler(app, core.NewJobRunner(app, core.NewJobRegistry()))
	if err != nil {
		return nil, err
	}

	if err := registerInfrastructureJobs(sc); err != nil {
		return nil, err
	}

	return sc, nil
}

// registerInfrastructureJobs holds the work that belongs to the process rather
// than to any feature. Keep it short: almost everything that looks like it
// belongs here belongs to a module instead, and the question to ask is "who
// decides when this changes".
func registerInfrastructureJobs(sc *core.Scheduler) core.IError {
	return sc.AddByDuration("worker.heartbeat", time.Minute, Heartbeat)
}

// Heartbeat says the scheduler is alive. It is also the smallest example of what
// a job is: an ordinary func(core.ICronjobContext) error with the same
// capabilities as an HTTP handler — c.DB(), c.Cache(), core.Requester(c),
// repositories, validation.
//
// A panic here becomes a logged error; it never takes the scheduler down.
func Heartbeat(c core.ICronjobContext) error {
	// c.Log() already carries job, run_id and attempt — passing them again only
	// prints them twice. Log what *this* job did, not what the runner knows.
	c.Log().Info("heartbeat")

	return nil
}
