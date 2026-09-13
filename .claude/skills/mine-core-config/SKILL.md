---
name: mine-core-config
description: Read, add or debug configuration in this mine-core service — .env vs APP_-prefixed OS variables, ENVConfig fields, module-specific keys via ctx.ENV(), opening a new backend in Bootstrap, docker-compose and CI. Use when adding a config key, when a setting is being ignored, or when wiring a new connection (cache, MQ, storage, mongo).
---

# Configuration

koanf reads `.env` and the OS environment into `core.IENV`. Get at it with
`ctx.ENV()` — `Config()` for the framework's own fields, `String/Int/Bool/Float64`
for anything else.

## The three traps

**1. `.env` keys carry no prefix; OS variables need `APP_`.**

```
ROLE=api          # in .env
APP_ROLE=api      # the same key from the OS environment — and it WINS
```

`APP_ROLE=all` written *inside* `.env` binds to `app_role` and is **silently
ignored**. This is how docker-compose points a container at the `db` host while
`.env` points at localhost for running the binary directly.

**2. `TEST_DATABASE_URL` is read from the process environment only** — not from
`.env`, no `APP_` prefix. In `.env` it does nothing and the suite silently stays
on sqlite. `make test-integration` sets it itself, for the throwaway postgres
[cmd/testpg](../../../cmd/testpg/) starts; one already exported wins and that
postgres is not started at all.

**3. Some keys default to `true`**, so they are read through `IENV.String`, not
as a bool field — an unset bool cannot be told from an explicit `false`.
`LOG_REQUEST`, `APP_SENTRY_CAPTURE_BODY`, `APP_SENTRY_SEND_ENV`,
`APP_SENTRY_ATTACH_STACKTRACE`.

## Framework keys

`env.Config()` (`*core.ENVConfig`) holds the ones the framework acts on:

```
ENV SERVICE HOST ROLE
LOG_LEVEL LOG_SIMPLE LOG_HOST LOG_PORT LOG_REQUEST
DB_CONNECTION_STRING | DB_DRIVER DB_HOST DB_PORT DB_NAME DB_USER DB_PASSWORD DB_SSLMODE
DB_LOG_LEVEL                 silent | error | warn (default) | info
CACHE_CONNECTION_STRING | CACHE_HOST …
MQ_CONNECTION_STRING | MQ_HOST …
DB_MONGO_* (DB_MONGO_NAME is always required — it is not taken from the URI)
S3_* / STORAGE_*   SMTP_*   FCM_*
JWT_SECRET
SENTRY_DSN SENTRY_RELEASE SENTRY_TRACES_SAMPLE_RATE … (see docs/sentry.md)
HTTP_LOG_LEVEL HTTP_LOG_BODY   # outgoing calls, independent of LOG_LEVEL
```

A URI wins over the discrete fields for the same backend. Full reference —
every key, its default, and the DSN rules: `$CORE/docs/env.md` (400 lines).

`DATABASE_URL` is **prisma's** own variable, not the framework's; both are set
because migrations and the service read different ones.

## A key that belongs to one module

Do not add a field to `ENVConfig` — read it ad hoc, in the module that owns it:

```go
const baseURLKey = "exchange_base_url"    // EXCHANGE_BASE_URL in .env, APP_EXCHANGE_BASE_URL from the OS

baseURL := s.ctx.ENV().String(baseURLKey)
if baseURL == "" {
    s.ctx.Log().Error("exchange rate provider is not configured",
        "key", baseURLKey, "hint", "set APP_EXCHANGE_BASE_URL, or EXCHANGE_BASE_URL in .env")
    return nil, emsgs.ExchangeNotConfigured
}
```

**Fail loudly and specifically when a required key is missing** — a
misconfiguration is one of the few things that earns `Log().Error`, because
nobody outside can fix it and nothing else reports it. Prefer refusing to a
silent default that quietly calls a third party nobody chose.

Document every new key in [.env.sample](../../../.env.sample), commented, with
what happens when it is unset. `.env` itself is not committed.

## Opening a new backend

[cmd/bootstrap.go](../../../cmd/bootstrap.go) builds the `App` once — config plus
one connection pool per **configured** backend, for the process lifetime:

```go
if cfg.CacheConnectionString != "" || cfg.CacheHost != "" {
    cache, err := core.NewCache(env)
    if err != nil { return nil, err }
    opts = append(opts, core.WithCache("default", cache))
}
```

A backend with no config is simply not opened, so the service runs on SQL alone.
Nothing returns nil for it: `ctx.Cache()` misses every read and drops every
write, so cache-aside code runs unchanged; `ctx.MQ()` and `ctx.Storage()` fail
every call naming the missing configuration — a message nobody receives is not
something to degrade quietly about.

Add a backend by copying that shape, then a key in `.env.sample`, the service in
[docker-compose.yml](../../../docker-compose.yml), and the CI variables in
[.github/workflows/ci.yml](../../../.github/workflows/ci.yml).

## In tests

```go
ctx := testkit.Context(t, coretest.WithEnv(map[string]string{"exchange_base_url": srv.URL}))
```

Scoped to that test, so nothing depends on the `.env` in anybody's checkout.
Never read a real `.env` value in a test.

## Roles

`ROLE` selects the composition root in [main.go](../../../main.go):
`api` (default) · `worker` · `all`. **`all` must run exactly one replica** — the
scheduler queue is in-process, so every replica fires every cron tick.
