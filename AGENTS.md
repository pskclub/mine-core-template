# AGENTS.md

The entry point for coding agents — Codex, Cursor, Copilot, Gemini CLI, Jules and
anything else that reads [agents.md](https://agents.md). Claude Code reads
[CLAUDE.md](CLAUDE.md) instead, and that is the file this one defers to.

**Read [CLAUDE.md](CLAUDE.md) first, in full, before changing anything.** It is
the canonical description of this service: the roles and the one composition
root, the request flow through route → handler → service → store, the import
rules that are enforced as a test, module and file naming, errors, logging,
configuration traps, and how the tests are run. Nothing in it is repeated here,
because two copies of a convention is how a repository ends up with two
conventions.

Longer form for any of it: [README.md](README.md), and
[modules/note/README.md](modules/note/README.md) as the worked example.

## Before you push

```bash
gofmt -w . && make lint
make test                # sqlite in memory
make test-integration    # a real postgres, started and thrown away by cmd/testpg
make postman             # only if a route changed — commit apispec/openapi.generated.json
```

## Skills

Each skill below is the decisions this repository already made about one subject,
short and with pointers to the file that demonstrates them. **Read the matching
one before working on that subject** — they answer the questions this codebase
answers differently from a generic Go service.

If your agent discovers skills by itself, it has already found them: they are
mirrored to `.codex/skills/`, the convention Codex uses. If it does not, this
table is the index — open the file whose row matches the task.

<!-- skills:start -->

| Skill | What it covers, and when to read it |
|---|---|
| [`mine-core-auth`](.claude/skills/mine-core-auth/SKILL.md) | Protect routes and work with the signed-in caller in this mine-core service — AuthRequire/AuthOptional/AuthRole, RegisterAuth wiring, the opaque bearer token scheme, roles, and ownership checks. Use when adding a protected route, changing who may call something, touching the auth module, or debugging a 401/403. |
| [`mine-core-config`](.claude/skills/mine-core-config/SKILL.md) | Read, add or debug configuration in this mine-core service — .env vs APP_-prefixed OS variables, ENVConfig fields, module-specific keys via ctx.ENV(), opening a new backend in Bootstrap, docker-compose and CI. Use when adding a config key, when a setting is being ignored, or when wiring a new connection (cache, MQ, storage, mongo). |
| [`mine-core-database`](.claude/skills/mine-core-database/SKILL.md) | Work with the database in this mine-core service — adding or changing a table (prisma migration + Go model + testkit registration), writing a module store on repository.Repo[M], scopes, pagination, transactions, soft vs hard delete, and the two GORM traps. Use whenever a schema, model, store or query changes. |
| [`mine-core-docs`](.claude/skills/mine-core-docs/SKILL.md) | Look up mine-core v2 framework API, behaviour, or documentation. Use when you need the real signature, option, or semantics of anything under github.com/pskclub/mine-core/v2 (core.*, valid.*, repository.*, coretest.*, utils.*, errmsgs.*, mongorepo.*), when a compile error mentions mine-core, when upgrading the mine-core version, or before guessing how a framework helper behaves. |
| [`mine-core-errors`](.claude/skills/mine-core-errors/SKILL.md) | Declare and return errors in this mine-core service — core.IError, the emsgs sentinels, framework errmsgs, wrapping with ctx.NewError, choosing a status, and mapping an upstream's failure to one of ours. Use when adding an error code, deciding a status, handling a repository or upstream failure, or when an error reaches a client in the wrong shape. |
| [`mine-core-http`](.claude/skills/mine-core-http/SKILL.md) | Write routes, handlers, request binding, pagination and responses in this mine-core service. Use when adding or changing an HTTP endpoint, binding path/query/body params, returning a list with paging, setting a status code, uploading a file, or regenerating the Postman collection. |
| [`mine-core-infra`](.claude/skills/mine-core-infra/SKILL.md) | Use mine-core's optional backends from this service — Redis cache (Remember, counters, locks, pub/sub), S3 storage and presigned uploads, RabbitMQ publish/consume, mailer and FCM push, plus health probes and graceful shutdown. Use when caching, storing a file, sending a message, an email or a notification, or wiring a new backend. |
| [`mine-core-jobs`](.claude/skills/mine-core-jobs/SKILL.md) | Add or change scheduled and background work in this mine-core service — cron jobs owned by a module, registering them with the scheduler, job params, retries, concurrency, cancellation, the worker/all roles, and testing a job. Use when writing a cron job, a cleanup or retention task, or anything that must run outside a request. |
| [`mine-core-logging`](.claude/skills/mine-core-logging/SKILL.md) | Add or review logging in this mine-core service — ctx.Log(), which level to use, which layers log at all, what the framework already logs, and what must never be logged. Use when adding a log line, when reviewing noisy or missing logs, or when deciding whether an error should be logged as well as returned. |
| [`mine-core-module`](.claude/skills/mine-core-module/SKILL.md) | Build or extend a feature module in this mine-core service — new module scaffold, new endpoint, new layer, wiring into cmd.Modules, and making one module use another. Use whenever adding a feature, a resource, or a use case, or when an arch import-rule test fails. |
| [`mine-core-requester`](.claude/skills/mine-core-requester/SKILL.md) | Call another service or a third-party API from this mine-core service — core.Requester, typed success and failure bodies, per-call timeouts, mapping an upstream's outcome to our own errors, and testing against an httptest server. Use whenever code needs to make an outbound HTTP request. |
| [`mine-core-review`](.claude/skills/mine-core-review/SKILL.md) | Review a change against this mine-core service's conventions before it is pushed or merged — layering, import rules, errors, ownership, validation placement, logging, tests, schema and config. Use when reviewing a diff or a merge request, or as a self-check after finishing a feature. |
| [`mine-core-testing`](.claude/skills/mine-core-testing/SKILL.md) | Write and run tests for this mine-core service — testkit.Context/Serve/SignIn, the sqlite vs postgres vs e2e tiers, module HTTP tests, service tests, faking an upstream, and what every module's test file must cover. Use when adding tests, when a test fails, or before pushing a change. |
| [`mine-core-validation`](.claude/skills/mine-core-validation/SKILL.md) | Write request validation with mine-core's valid package — Valid/Validate methods, field rules, uniqueness and existence checks against the database, conditional and cross-field rules, nested and array payloads, custom codes and messages. Use when adding or changing a request struct, or when deciding whether a rule belongs in the request or the service. |
| [`mine-core-workflow`](.claude/skills/mine-core-workflow/SKILL.md) | Run, build, migrate and ship this mine-core service — make targets, docker compose, live reload, the prisma migration loop, roles, lint and the CI pipeline. Use when starting the service locally, running a migration, debugging a build or a private-module fetch, or checking what CI will do to a change. |

<!-- skills:end -->

The table and the `.codex/skills/` mirror are both generated by `make skills`
from `.claude/skills/`, which is where the skills are actually written. Edit a
skill there and run `make skills`; CI runs `make skills-check` and fails if the
two have drifted.
