---
name: mine-core-workflow
description: Run, build, migrate and ship this mine-core service — make targets, docker compose, live reload, the prisma migration loop, roles, lint and the CI pipeline. Use when starting the service locally, running a migration, debugging a build or a private-module fetch, or checking what CI will do to a change.
---

# Local development and shipping

## Every day

```bash
make start          # docker compose: api + postgres + redis
make dev            # api with live reload (air) — the usual loop
make run            # api role, no reload
make logs           # docker logs -f api
make stop
```

```bash
make test           # unit + module + arch, sqlite in memory
make lint           # go vet + gofmt -l + golangci-lint
gofmt -w .          # `make lint` only PRINTS offenders; CI fails on them
```

A single test: `go test ./modules/note/... -run TestNoteAPI_crud -v`, plus
`-count=1` to defeat the cache.

## Roles

One binary, one role per deployment. [main.go](../../../main.go) reads `ROLE`
and dispatches:

```bash
make run          # api    — HTTP only (default)
make run-worker   # worker — scheduled jobs only
make run-all      # all    — both, ONE replica only (in-process cron queue)
```

To scale the API, run `api` replicas plus a single `worker`.

## Modules

mine-core is a public module, so there is nothing to configure.

```bash
make install    # go mod download
```

`make lint` runs golangci-lint through `go run` at the version CI pins, so it
needs nothing installed either.

## The migration loop

Prisma owns the schema; **nothing in Go migrates the database.**

```bash
# 1. edit prisma/schema/<module>.prisma
make migrate-dev    # author a migration on the host (prompts for a name, needs a shadow db)
# 2. add the struct in models/
# 3. register it in allModels() in testkit/testkit.go   <-- easy to forget
make generate       # only if prisma/seed.ts touches it
make migrate        # apply committed migrations in a container — the same step CI runs
make seed
make studio
```

Full rules: `mine-core-database`.

## Before pushing

```bash
gofmt -w . && make lint
make test
make test-integration   # the real prisma schema on a postgres cmd/testpg starts and throws away
make postman            # if a route changed
make skills             # if a skill's name or description changed
```

`make test-e2e` needs a **running** service (`make start` first) — it talks to
the deployed binary over the network, behind the `e2e` build tag.

## What CI does

[.github/workflows/ci.yml](../../../.github/workflows/ci.yml), on GitHub Actions:

- **lint** — `gofmt -l`, `go vet`, `golangci-lint run`, then the two generated
  artefacts: `make api-check` (the committed OpenAPI document) and
  `make skills-check` (`.codex/skills` and the AGENTS.md index, generated from
  `.claude/skills` by `make skills`). Separate from test so a formatting nit and
  a broken test do not arrive as one red pipeline.
- **test (sqlite)** — `go build`, `go test ./...`.
- **test (postgres)** — the same suite with a postgres service container and
  `TEST_DATABASE_URL` set in the job's `env` — it is read straight from the environment, not through `.env`,
  with no `APP_` prefix.
- **docker build** — each Dockerfile, built and never pushed.
- **CI OK** — one check that passes only when all of the above did; it is the
  one to require in branch protection.

The import rules are **not** a CI tool: they are
[arch/arch_test.go](../../../arch/arch_test.go) and run with the rest of the
suite, so they need nothing installed and fail with an explanation.

There are no push or deploy jobs — add them for your registry and cluster in a
real service.

## Containers

`docker-compose.yml` runs **api**, **db** (postgres 17) and **cache** (redis),
plus one-shot `migration` and `seed` services. The container gets its config
from `APP_`-prefixed OS variables, which is why they win over `.env` — see
`mine-core-config`. `Dockerfile` builds the service; `migrate.Dockerfile` and
`seed.Dockerfile` run prisma.

## Scaffolding

```bash
make new-module name=order
```

Writes the whole module shape and prints the four things it cannot decide. See
`mine-core-module`.
