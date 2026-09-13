# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Service template built on **mine-core v2** (`github.com/pskclub/mine-core/v2` — Echo v5, GORM, koanf, slog, Sentry). Go 1.27. Prisma owns the database schema; Go owns everything else.

[README.md](README.md) is the long-form rationale for every convention below, and [modules/note/README.md](modules/note/README.md) is the worked example of a module. Read the relevant section before changing a pattern rather than inferring intent from one file.

[.claude/skills/](.claude/skills/) holds one skill per subject — module layout, HTTP, validation, errors, database, auth, jobs, testing, requester, infra, config, logging, workflow, review, and `mine-core-docs` for reaching the framework's own documentation. They load on their own when a task matches; [.claude/skills/README.md](.claude/skills/README.md) is the index. A convention change should update the matching skill in the same commit.

That directory is the only copy, and it serves every agent: `make skills` mirrors each skill to `.codex/skills/` as a stub pointing back here, and regenerates the index in [AGENTS.md](AGENTS.md) — which is the entry point for anything that does not read this file. Run it when a skill's `name` or `description` changed; CI runs `make skills-check`.

## Commands

```bash
make install            # go mod download
make start              # docker compose: api + postgres + redis
make run                # api role       (make run-worker / run-all for the others)
make dev                # api with live reload   (make dev-worker / dev-all for the others)
make test               # unit + module + arch tests, sqlite in memory
make test-integration   # same suite on a real postgres cmd/testpg starts and throws away
make test-e2e           # against a *running* service (build tag `e2e`)
make lint               # go vet + gofmt -l + golangci-lint
make migrate-dev        # author a prisma migration (host, prompts for a name)
make migrate            # apply committed migrations (container; same step as CI)
make new-module name=order
make postman            # regenerate data/postman_collection.generated.json
make skills             # regenerate .codex/skills and the AGENTS.md index from .claude/skills
```

A single test: `go test ./modules/note/... -run TestNoteAPI_crud -v`. Add `-count=1` to defeat the cache.

`make lint` runs golangci-lint through `go run` at the version CI pins, so it needs nothing installed. Note that `make lint` only *prints* gofmt offenders; CI fails on them, so run `gofmt -w .` before pushing.

Every recipe in the [Makefile](Makefile) runs on Windows, macOS and Linux: on Windows make uses `cmd.exe` unless an `sh.exe` is on PATH, so a recipe carries no `VAR=value command` prefix (that is POSIX syntax — the role and the test URLs are target-specific `export`s instead), no `chmod`, and no line continuations.

`make dev` is `go tool air`: air is a tool dependency in go.mod, pinned there like postmangen, so live reload needs no install step. [.air.toml](.air.toml) is written for all three platforms — `[build]` for linux/macOS, `[build.windows]` overriding the two values that need `.exe`. Bump it with `go get -tool github.com/air-verse/air@<version>` and check the version's own `go` directive first: `go get -tool` raises this module's to match.

## Architecture

**One binary, one role per deployment.** [main.go](main.go) reads `role` from config and dispatches to `cmd.APIRun` / `cmd.WorkerRun` / `cmd.AllRun`. `all` must never run more than one replica — the scheduler queue is in-process, so every replica fires every cron tick.

**One composition root, in [cmd/](cmd/):**

- [cmd/modules.go](cmd/modules.go) `Modules` — the only function that knows every module and satisfies every module's dependencies. It returns a `*core.ModuleSet`; each module declares what it attaches in its own directory (`core.IModule` on the type in `<name>.module.go`), and `core.RunModules` attaches it to whatever the role has — cron only where there is a scheduler, routes only where there is a server. One list, every role. Tests assemble the same set (`testkit.Serve`), so a module wired wrongly fails a test.
- [cmd/api.go](cmd/api.go) `NewAPI` — the probes (handed `mods.HealthChecks()`) and this deployment's HTTP options. It knows no module.
- [cmd/worker.go](cmd/worker.go) `newScheduler` — only the jobs that belong to the *process* rather than to a feature. A module arms its own through `Module.Cron`.

[cmd/inspect.go](cmd/inspect.go) `mountInspectors` — `/_docs` (API reference, from the OpenAPI document embedded in [apispec/](apispec/)) and `/_dev` (inspector: capabilities, routes, config, jobs, **modules**, health). On when `ENV=dev`, or anywhere `DEVTOOLS_PASSWORD`/`APIDOCS_PASSWORD` is set; off otherwise, routes and all. Called from `serve`, not `NewAPI` — they are a deployment concern, so tests get neither.

[cmd/bootstrap.go](cmd/bootstrap.go) builds the `App` once: config plus one connection pool per configured backend, for the process lifetime. A backend with no config is simply not opened.

**Request flow:** route (`modules/<name>/<name>.http.go`) → handler (bind, convert, call, render — nothing else) → service (business rules, takes any `core.IContext`) → store (the module's own repository). A job runs the same way: `Cron` in `<name>.jobs.go` arms a `core.ICronjobContext` entry point in `handler/` ([auth.job.go](modules/auth/handler/auth.job.go)), which converts and calls the service — so `service/` never learns what triggered it.

Every layer returns `core.IError`; a handler returns it as-is and the server renders `{code, message, fields}` with the error's status. Wrap repository errors with `s.ctx.NewError(err, err)` — that preserves the status and reports 5xx to Sentry with request/user scope.

Services take `core.IContext`, never a concrete context type, so the same code runs under HTTP, a job, or a test with no test-only interfaces.

## The import rules — enforced as a test

[arch/arch_test.go](arch/arch_test.go) runs under `make test` and fails the build on:

1. `models/` and `consts/` import nothing from this repository.
2. `repo/`, `emsgs/`, `helpers/`, `middlewares/` import no module and no `cmd`.
3. No module imports another module.
4. No module imports `cmd`.
5. A module is entered through its **top-level package only** — nothing outside `modules/note` may import `modules/note/service`.
6. `testkit` is imported only from `_test` files.

Consequences to work with rather than around:

- A module that needs another declares the shape it needs as **its own interface** ([modules/auth/service/auth.deps.go](modules/auth/service/auth.deps.go)) and `cmd.Modules` supplies it. What a module offers others goes in `<name>.api.go` as type aliases plus one-line forwards ([modules/user/user.api.go](modules/user/user.api.go)).
- `middlewares` cannot import auth (every module's routes import `middlewares`), so `cmd.Modules` hands it the token lookup at startup via `middlewares.RegisterAuth`. The guard is keyed per-`App`, which is what lets parallel tests each have their own.
- A module's *external* test package (`package note_test`) may import siblings; only non-test imports are checked.

## Module conventions

- File names mirror their directory: `note.handler.go` in `handler/`, `note.store.go` in `store/`. Things that are a different kind keep their own name (`note.request.go`, `note.dto.go`).
- The module's own directory holds every point where it attaches to the service, **one file per kind**: `<name>.module.go` (the module itself), `<name>.http.go` (routes), `<name>.jobs.go` (`Jobs`/`Cron`), `<name>.mq.go` (`Consumers`), `<name>.api.go` (what other modules may call). The guarantee is that nothing it registers lives outside the directory — not that there is one file; a module with a single kind may keep it in `<name>.module.go` ([modules/home](modules/home/home.module.go)).
- Routes are written in full, **one middleware per line** — `e.GET("/notes/:id", c.Find, middlewares.AuthRequire(e))`. No groups, no prefixes. The safety net is each module's `Test<Name>API_requiresAuthentication`, which names every route and asserts an anonymous caller gets 401. **Add every new route to that test.**
- Payload rules go in the request's `Valid` (answerable from the request alone); rules that need rows go in the service and return an `emsgs` error.
- Ownership is threaded as an explicit first argument (`ownerID string`) and applied as a **scope/WHERE clause**, re-applied on every statement. "Not yours" answers 404, never 403.
- Handlers convert request → payload with `utils.Copy[service.CreatePayload](input)`.
- `c.GetPageOptionsWithAllowed("created_at", ...)` is what makes `order_by` safe.
- A module has only the layers it has work for — [modules/home](modules/home/home.module.go) is one file; [modules/exchange](modules/exchange/) has no `store/` because its data is an HTTP call.

`apispec/openapi.generated.json` is **committed** (go:embed needs it at compile time) and `make api-check` — run by CI — fails when it no longer matches the routes. Run `make postman` after touching a route and commit what changes.

`make new-module name=order` writes the whole shape and prints the four things it cannot decide: the prisma schema, the model, the `allModels()` line, and the line in `cmd.Modules`.

## Schema and models

Nothing in Go migrates the database. A table change is: edit `prisma/schema/*.prisma` → `make migrate-dev` → add the matching struct in [models/](models/) → **register it in `allModels()` in [testkit/testkit.go](testkit/testkit.go)** so the sqlite suite builds the table → `make generate` if the seed touches it.

## Errors

This service's errors live in [emsgs/](emsgs/), one file per module, declared once as sentinels (compare with `errors.Is`). Framework errors (`NotFound`, `BadRequest`, `Unauthorized`, `DBError`) come from mine-core's `errmsgs` — use those directly. Statuses come from `net/http`, never a literal. A validation code raised with `v.Must` needs a message registered in that file's `init`.

## Logging

`ctx.Log()` is taken at the point of use, never stored on a struct — it already carries the request id (or job name, run id, attempt). Key-value pairs, never `fmt.Sprintf`.

Already logged by the framework, so do not repeat: method/path/status/latency/request-id (request middleware), every 5xx with user and scope (Sentry, from `ctx.NewError`), failed and slow SQL plus every statement at `LOG_LEVEL=debug` (GORM logger), a job's start/failure/retry, panics. **An error that is returned is not also logged.** Handlers and stores are silent by design.

Levels: **Info** for a write that changed data and for every scheduled run (count zero included); **Warn** for the service correctly refusing a caller (this is what you alert on by rate); **Error** only for what nobody else reports — in this repo exactly two cases, a route registered without its middleware and `RegisterAuth` never called; **Debug** for anything that runs on every request.

Never log tokens or their digests, password hashes, or personal data by value — `user_id`, not the email; `note_id`, not the note.

## Configuration traps

- Keys in `.env` carry **no prefix** (`ROLE=api`). The same key via the OS environment needs `APP_` (`APP_ROLE=api`) and wins. `APP_ROLE=all` written inside `.env` binds to `app_role` and is silently ignored.
- `TEST_DATABASE_URL` is read from the **process environment only** — not from `.env`, no `APP_` prefix. In `.env` it does nothing and the suite silently stays on sqlite.
- Module-specific keys are read ad hoc: `ctx.ENV().String("exchange_base_url")`.

## Testing

`make test` gives every test its own `:memory:` sqlite database. `make test-integration` gives every test a temporary postgres schema built from `prisma/schema/migrations` — the SQL production runs. Run it before pushing: sqlite has no partial indexes, no UUID type and no `ILIKE`, so it cannot reject a schema postgres would.

**`make test-integration` needs nothing installed.** [cmd/testpg](cmd/testpg/) fetches a real postgres 17 (once, cached under `$HOME/.embedded-postgres-go`), runs it on a private port for the length of one `go test` and deletes the cluster after — no docker, no daemon. It sets `TEST_DATABASE_URL` in the environment of the `go test` it spawns, so the suite itself needed no change to use it. `ARGS` passes flags and packages through: `make test-integration ARGS='-run TestNoteAPI_crud ./modules/note/...'`, and `ARGS='-pglog'` shows the server log.

**A `TEST_DATABASE_URL` already exported wins and testpg starts nothing** — that is how the same target runs against the docker-compose postgres, a shared database, or CI's service container.

Both paths go through [testkit/testkit.go](testkit/testkit.go):

```go
ctx := testkit.Context(t)               // isolated db, service-level tests
anon, app := testkit.Serve(t)           // the whole service on a real listener, from cmd.Modules
c := testkit.SignIn(t, anon)            // a client carrying a real token
```

Fake an upstream HTTP dependency with `httptest.Server` and point config at it (`coretest.WithEnv`), rather than mocking `IRequester` — see [modules/exchange/service/exchange.service_test.go](modules/exchange/service/exchange.service_test.go).

## Calling other services

Always `core.Requester(ctx)` — never `http.Get`, never a client built for the occasion. It carries the caller's deadline and trace and shares one connection pool. Use `SetResult`/`SetError` so both the success and failure bodies decode into our own types. Then **map every upstream outcome to an error this service owns**; an upstream code must never reach our clients. See [modules/exchange/service/exchange.service.go](modules/exchange/service/exchange.service.go) and the mapping table in the README.

## Two GORM traps

- **`Updates` with a struct skips zero values** — `pinned` going `true` → `false` is silently dropped. Any column that can legitimately hold `false`/`0`/`""` must be written with a `map[string]any`.
- **Use `repo.DefaultOrder(opts, ...)`, not `.Order()`** — `Pagination` already applies `opts.OrderBy`, so adding `Order()` emits the clause twice.

## Authentication

Opaque bearer tokens, verified against the database; only the SHA-256 digest is stored. Plain SHA-256 (not bcrypt) is deliberate for tokens — 256 bits of `crypto/rand`, looked up on every request. Passwords use bcrypt. Revocation is a `DELETE`; TTL is `TokenTTL` in [modules/auth/service/auth.service.go](modules/auth/service/auth.service.go) (24h, not re-exported). Sign-in answers identically for a wrong password and an unknown address. `c.GetUser()` inside a protected handler returns the caller; `middlewares.AuthOptional` and `middlewares.AuthRole` exist for the other two cases.
