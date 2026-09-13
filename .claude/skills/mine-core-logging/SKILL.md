---
name: mine-core-logging
description: Add or review logging in this mine-core service — ctx.Log(), which level to use, which layers log at all, what the framework already logs, and what must never be logged. Use when adding a log line, when reviewing noisy or missing logs, or when deciding whether an error should be logged as well as returned.
---

# Logging

```go
s.ctx.Log().Info("note created", "note_id", note.ID, "owner_id", ownerID)
```

`ctx.Log()` is taken **at the point of use, never stored on a struct** — it
already carries the request id, or the job name, run id and attempt. A stored
copy adds nothing and loses that binding.

**Key–value pairs, never `fmt.Sprintf`.** The whole point of structured logging
is that `owner_id` can be searched for.

`log := s.ctx.Log().With("base", base, "quote", quote)` when several lines in one
function share fields.

## Do not repeat what the framework already logs

- method, path, status, latency, request id — the request middleware
- every 5xx, with user and scope — Sentry, via `ctx.NewError`
- failed and slow SQL always, every statement at `LOG_LEVEL=debug` — the GORM logger
- a job's start, failure and retry — the job runner
- panics — recover middleware
- outgoing HTTP calls — the requester's own log (`HTTP_LOG_LEVEL`)

**An error that is returned is not also logged.** A line beside a `return` is the
same failure written twice, and the second copy has less context than the first.

## Which layers log

| Layer | Logs? |
|---|---|
| `store/` | **No.** The GORM logger covers it, with the SQL and the timing. |
| `handler/` | **No** — except the one case below. |
| `service/` | **Yes.** This is where a line knows what it *meant*. |
| `job` | Yes — one line per run (see below). |
| wiring | Only a misconfiguration. |

The handler exception: reaching a protected handler with no caller means a route
was registered without its middleware, which the 401 alone would hide as an
ordinary unauthenticated request. See `callerID` in
[note.handler.go](../../../modules/note/handler/note.handler.go).

## Levels

**Info** — a write that changed data, and every scheduled run **including the
ones that did nothing**. `count=0` every hour is the proof the job is alive; a
job that only speaks when it acts is indistinguishable from one that has stopped.

**Warn** — the service correctly refusing a caller, or a dependency misbehaving
in a way we handled. This is the level you alert on **by rate**: it is normal
singly, a problem in bulk.

```go
s.ctx.Log().Warn("note rejected", "reason", "limit_reached", "owner_id", ownerID, "count", count)
```

**Error** — only what nobody else reports and nobody outside can fix. In this
repository that is exactly two cases: a route registered without its middleware,
and `RegisterAuth` never called — plus a missing required config key. If Sentry
or the job runner will report it, this is not it.

**Debug** — anything that runs on every request, where the answer is useful only
when something looks wrong.

```go
s.ctx.Log().Debug("notes listed", "owner_id", ownerID, "total", page.Total, "q", opts.Q)
```

## Never log

Tokens or their digests · password hashes · personal data by value.

**Ids, not contents**: `user_id`, not the email; `note_id`, not the note's title
or body. What a user wrote is theirs, and a log line is the wrong place for it.

`HTTP_LOG_BODY` is off by default for the same reason — a body carries
credentials and personal data, and is the easiest way to turn a log store into a
place secrets live.

## In a job

`c.Log()` already carries `job`, `run_id` and `attempt`. Log what *this run did*:

```go
c.Log().Info("purged expired access tokens", "count", removed, "cutoff", cutoff)
```

## Configuration

`LOG_LEVEL` (`debug` in dev) · `LOG_SIMPLE=true` for human-readable output ·
`DB_LOG_LEVEL` and `HTTP_LOG_LEVEL` override SQL and outgoing-call verbosity
independently of `LOG_LEVEL`.

Long form: `$CORE/docs/logging-practices.md`, `$CORE/docs/logger.md`.
