---
name: mine-core-jobs
description: Add or change scheduled and background work in this mine-core service — cron jobs owned by a module, registering them with the scheduler, job params, retries, concurrency, cancellation, the worker/all roles, and testing a job. Use when writing a cron job, a cleanup or retention task, or anything that must run outside a request.
---

# Scheduled jobs

A job is an ordinary `func(core.ICronjobContext) error` with the **same
capabilities as an HTTP handler**: `c.DB()`, `c.Cache()`, `core.Requester(c)`,
repositories, validation, `c.Log()`.

## A module owns its jobs

Jobs live in the module whose data they touch, for the same reason a module owns
its tables. How long a token's row is kept is an auth decision; in a shared
`jobs` package it would be a decision anyone can change and nobody owns.

Three files, each answering one thing:

```go
// modules/auth/auth.jobs.go — what this module puts on a clock
func (*Module) Cron(sc *core.Scheduler) core.IError {
    return sc.AddByDuration("auth.purge-expired-tokens", purgeInterval, handler.PurgeExpiredTokens)
}
```

```go
// modules/auth/handler/auth.job.go — the entry point
//
// A job entry point is a handler, not a service method: it reads the trigger's
// input (here, the clock), converts it, calls the service and records the
// result — the same four steps an HTTP handler does.
func PurgeExpiredTokens(c core.ICronjobContext) error {
    cutoff := time.Now().UTC().Add(-service.TokenRetention)

    removed, err := service.PurgeTokensExpiredBefore(c, cutoff)
    if err != nil {
        return err              // returned, not logged — the runner logs failed runs
    }

    c.Log().Info("purged expired access tokens", "count", removed, "cutoff", cutoff)
    return nil
}
```

```go
// modules/auth/service/auth.job.go — the work, against core.IContext only
//
// The cutoff is a parameter rather than read from the clock inside, so the rule
// can be tested at a fixed instant instead of by waiting. service/ never learns
// whether a request or a scheduler asked.
func PurgeTokensExpiredBefore(ctx core.IContext, cutoff time.Time) (int64, core.IError)
```

Arming it is a method on the module, so the job cannot be attached without the
module and the module cannot be attached without the job — a feature cannot be
half-registered.

`Cron` is only called in a role that has a scheduler, so an `api` replica serves
the module's routes and arms nothing, out of the same module list. Nothing in
`cmd/` changes when a module gains its first job.

**Job names are prefixed with the module** (`auth.purge-expired-tokens`). They
are stable identifiers — they label runs in the job store, the logs and Sentry
cron monitors — so the prefix is what stops two teams both picking "cleanup".

## Registering

```go
sc.AddByDuration("mod.thing", time.Hour, Fn)     // every hour
sc.AddByCron("mod.thing", "0 3 * * *", Fn)       // 03:00 daily
sc.Add(core.JobDef{...}, Fn)                     // everything else
```

`JobDef` is where retries, timeouts and concurrency are declared:

```go
core.JobDef{
    Name:          "billing.settle",
    Description:   "settle yesterday's orders",
    Schedule:      ...,               // nil = manual only
    Timeout:       5 * time.Minute,   // the run context is cancelled after it
    MaxAttempts:   3,                 // total tries; 0 or 1 = no retry
    Backoff:       nil,               // nil = exponential 1s → 5m
    MaxConcurrent: 1,                 // 1 = singleton
    Replayable:    utils.ToPointer(false),  // false for payments, real emails
    RetainRuns:    30 * 24 * time.Hour,
}
```

## Writing the body

**Take the clock as a parameter, not from inside.** The interesting property —
"it removes what is past the retention window and nothing else" — is then
testable at a fixed instant instead of by waiting:

```go
func PurgeTokensExpiredBefore(ctx core.IContext, cutoff time.Time) (int64, core.IError)
```

Note the signature takes `core.IContext`, not `ICronjobContext`: the rule is
callable from a test, a handler or an admin tool.

**Long loops must check for cancellation** — Go cannot kill a goroutine:

```go
for _, item := range items {
    if c.IsStopping() {
        return c.Err()
    }
    ...
}
```

**Parameters** are validated with the same builder as an HTTP payload, and
violations fail the run before the handler is called:

```go
func (p *ReportParams) Valid(ctx core.IContext) core.IError {
    v := valid.New(ctx)
    v.Str("date", p.Date).Required().Date()
    return v.Error()
}

func Report(c core.ICronjobContext) error {
    var p ReportParams
    if err := c.Params(&p); err != nil {
        return err
    }
    c.Progress(50, "halfway")
    c.SetResult(summary)
    return nil
}
```

Also on the context: `c.JobName()`, `c.RunID()`, `c.Attempt()`, `c.Trigger()`.

## Logging in a job

`c.Log()` already carries `job`, `run_id` and `attempt` — passing them again
only prints them twice. Log what *this run did*.

**Every run writes one Info line, including the ones that did nothing.** A job
that only speaks up when it acts is indistinguishable from a job that has stopped
running; `count=0` every hour is the proof it is alive. A failure is `return`ed,
never logged — the runner logs and reports it with the name, run id and attempt.
A panic becomes a logged error and never takes the scheduler down.

## Running it

```bash
make run          # api role — no jobs
make run-worker   # APP_ROLE=worker — jobs only
make run-all      # APP_ROLE=all — both, ONE replica only
```

⚠️ The default scheduler queue is **in-process**, so `all` with N replicas fires
every cron tick N times. To scale the API, run `api` replicas plus a single
`worker` ([cmd/all.go](../../../cmd/all.go)).

`newScheduler` hands the runner to `core.Runner` explicitly so shutdown drains
jobs at the right point in the sequence, with this service's `shutdownTimeout`.

## Testing

```go
j := coretest.NewJob(t, coretest.WithAutoMigrate(...))
j.Register("auth.purge-expired-tokens", handler.PurgeExpiredTokens)
run := j.Run("auth.purge-expired-tokens", nil)
require.Equal(t, core.RunSucceeded, run.Status)   // RunFailed, RunSkipped, RunCanceled
```

Prefer testing the pure rule directly with a fixed cutoff —
[auth.job_test.go](../../../modules/auth/service/auth.job_test.go) — and use the
runner only when the run's own machinery (params, retry, status) is the subject.

Long form: `$CORE/docs/jobs.md` (581 lines), `scheduler.md`, `api-with-cron.md`,
`testing-jobs.md`.
