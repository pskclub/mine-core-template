---
name: mine-core-docs
description: Look up mine-core v2 framework API, behaviour, or documentation. Use when you need the real signature, option, or semantics of anything under github.com/pskclub/mine-core/v2 (core.*, valid.*, repository.*, coretest.*, utils.*, errmsgs.*, mongorepo.*), when a compile error mentions mine-core, when upgrading the mine-core version, or before guessing how a framework helper behaves.
---

# Reading mine-core itself

mine-core ships ~16,000 lines of documentation and its full source inside the
module cache. **Never guess a signature — read it.** The same docs are on
[pkg.go.dev](https://pkg.go.dev/github.com/pskclub/mine-core/v2) and GitHub, but
the copy in the module cache is the version pinned in go.mod.

## Locate it

```bash
go list -m -f '{{.Dir}}' github.com/pskclub/mine-core/v2
```

Prints e.g. `/home/you/go/pkg/mod/github.com/pskclub/mine-core/v2@v2.2.0`.
Everything below is relative to that directory (`$CORE`). Files there are
read-only — never edit them.

The version is pinned in [go.mod](../../../go.mod). If the docs disagree with
the compiler, the compiler is right and the doc belongs to another version:
check the
[release notes](https://github.com/pskclub/mine-core/releases) for the version in go.mod.

## The doc map — `$CORE/docs/`

| Need | Read |
|---|---|
| Wiring an App, `IContext`, scoped data | `context.md`, `getting-started.md`, `lifecycle.md` |
| Config keys, koanf, connection strings | `env.md` (the big one — 400 lines) |
| `IError`, `errmsgs`, recover | `error-handling.md`, `service-errors.md` |
| Routes, binding, groups, `IHTTPContext` | `http.md` |
| Validation rules | `validation.md` |
| Bearer auth, roles, `c.GetUser()` | `auth.md` |
| GORM connection, DSN | `database.md`, `database-connections.md` |
| `repository.Repo[M]` | `repository.md`, `-queries`, `-writes`, `-relations`, `-recipes`, `-testing` |
| Pagination, `PageOptions`, `Page[M]` | `database-pagination.md` |
| Transactions | `database-transactions.md` |
| Redis | `cache.md`, `-operations`, `-counters`, `-locks`, `-patterns`, `-testing` |
| Outbound HTTP | `requester.md` |
| S3 | `storage.md`, `-setup`, `-upload`, `-download`, `-presign`, `-patterns` |
| Cron / job runner / params / queue | `jobs.md` (581 lines), `scheduler.md`, `api-with-cron.md` |
| RabbitMQ / redis fan-out | `mq.md`, `pubsub*.md` |
| Mongo | `mongo*.md`, `mongo-repository*.md` |
| Mail, FCM | `mailer.md`, `push.md` |
| Logging | `logger.md`, `logging-practices.md` |
| Sentry | `sentry.md` (436 lines) |
| Health probes, graceful shutdown | `health.md`, `runner.md`, `deployment.md` |
| Test fixtures | `testing.md` + `testing-{unit,integration,e2e,mock,fixtures,database,http,jobs,assertions}.md` |
| Postman generation | `postman.md` |
| JWT, CSV, pointer/slice helpers | `jwt.md`, `csv.md`, `utils.md` |
| Recommended layout | `structure.md` |

## When the doc is not enough

The source is right there and is the authority:

```bash
CORE=$(go list -m -f '{{.Dir}}' github.com/pskclub/mine-core/v2)
grep -n "^type I[A-Za-z]* interface" "$CORE"/*.go        # every interface
sed -n '/^type IHTTPContext interface/,/^}/p' "$CORE/http_context.go"
grep -h "^func " "$CORE/repository/base.repo.go"          # the Repo[M] surface
grep -h "^func " "$CORE/valid"/*.go                       # every validation rule
ls "$CORE/examples/"                                      # runnable examples
```

`*_test.go` next to each file is the best behavioural spec — `cache_test.go`,
`job_runner_test.go`, `http_routing_test.go` show the intended usage.

## Upgrading the version

```bash
go get github.com/pskclub/mine-core/v2@vX.Y.Z
make test && make lint
```

Read the new version's
[release notes](https://github.com/pskclub/mine-core/releases) first — a breaking
change there comes with its migration steps. `go tool postmangen` is pinned by the
`tool` directive in go.mod, so bumping core also changes the generator.
