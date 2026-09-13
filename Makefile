# Every recipe here runs on Windows, macOS and Linux. Two rules keep it that way,
# because on Windows make runs recipes through cmd.exe unless a sh.exe happens to
# be on PATH — so a recipe must not assume either shell:
#
#   - No `VAR=value command` prefixes. That is POSIX shell syntax; cmd.exe reads
#     it as the name of a program. Use a target-specific `export` instead — make
#     puts the variable in its own environment, and the child process inherits it
#     whichever shell make picked.
#   - Nothing shell-specific inside a recipe: no `chmod` or `source`, no
#     backslash continuations, no quoting tricks. `&&` is the one operator both
#     shells agree on; anything longer becomes a second recipe line.
#
# (Directives outside a recipe — `.PHONY` below, variable assignments — are make's
# own syntax and are read by make itself, so they may span lines.)

# None of these targets produce a file of their own name.
.PHONY: start start-build stop restart restart-build \
	run run-worker run-all dev dev-worker dev-all \
	install logs migrate-dev migrate seed generate studio \
	test test-integration test-e2e lint new-module postman postman-flat api-check \
	skills skills-check

start:
	docker compose up -d

start-build:
	docker compose up -d --build

stop:
	docker compose down

restart:
	$(MAKE) stop
	$(MAKE) start

restart-build:
	$(MAKE) stop
	$(MAKE) start-build

run:
	go run main.go

# Scheduled jobs only — the same binary in its worker role.
run-worker: export APP_ROLE = worker
run-worker:
	go run main.go

# API and jobs in one process. Single instance only — see cmd/all.go.
run-all: export APP_ROLE = all
run-all:
	go run main.go

# --- Live reload --------------------------------------------------------------
# Rebuild and restart the service on every save. air does the watching; what it
# builds and how it stops the old process is in .air.toml.
#
# air is a tool dependency in go.mod, exactly like postmangen, so there is
# nothing to install first and everybody runs the version the repo pins:
#
#   go get -tool github.com/air-verse/air@v1.67.1   # how it got there
#
# Pin bumps go through that same command. Check the version's own `go` directive
# first — air v1.67.3 declares go 1.26, and `go get -tool` would rewrite this
# module's directive to match, putting every developer and CI on a new toolchain
# for the sake of the dev loop.
dev:
	go tool air -c .air.toml

# The same loop for the other two roles — see the note at the top on why the role
# is an `export` and not a prefix.
dev-worker: export APP_ROLE = worker
dev-worker:
	go tool air -c .air.toml

dev-all: export APP_ROLE = all
dev-all:
	go tool air -c .air.toml

# Fetch every module go.mod needs. mine-core is public, so there is nothing to
# configure first.
install:
	go mod download

logs:
	docker logs -f api

# --- Schema: prisma owns it ---------------------------------------------------
# Every table change starts in prisma/schema/*.prisma. The Go models describe
# tables prisma created; nothing in Go migrates the database.
#
# Authoring runs on the host against the postgres published on localhost, because
# `migrate dev` prompts for a migration name and needs a shadow database.
migrate-dev:
	bun run migrate

# Apply committed migrations. Containerised, so CI and every developer run the
# identical step.
migrate:
	docker compose run --rm --build migration

seed:
	docker compose run --rm --build seed

# Regenerate @prisma/client after a schema change (seed.ts depends on it).
generate:
	bun run generate

studio:
	bun run studio

# sqlite in memory: no dependencies, fast enough to run on every save.
test:
	go test ./...

# The same tests against a real postgres, which cmd/testpg starts and throws
# away itself — no docker, no daemon, nothing installed. It fetches a real
# postgres 17 (once, cached under $HOME), runs it on a private port for the
# length of one `go test`, and hands the suite its DSN through
# TEST_DATABASE_URL, which is the only switch testkit needs. Each test still
# gets a temporary schema built from prisma/schema/migrations — the SQL
# production runs — and the cluster is deleted when the run ends.
#
# Run this before pushing: sqlite has no partial indexes, no UUID type and no
# ILIKE, so it cannot reject a schema that postgres would.
#
# ARGS passes flags and packages through:
#
#	make test-integration ARGS='-pglog'                      # show the server log
#	make test-integration ARGS='./modules/note/...'
#	make test-integration ARGS='-run TestNoteAPI_crud ./modules/note/...'
#
# The quotes are your shell's, not make's: from cmd.exe use "ARGS=-run TestX ./..."
# instead, since cmd.exe does not strip single quotes and they would reach go test.
#
# To run against a postgres you already have — the one from `make start`, or a
# shared database — export TEST_DATABASE_URL in your own shell and testpg starts
# nothing and uses that instead:
#
#	TEST_DATABASE_URL=postgres://my_user:my_password@localhost:5432/my_database?sslmode=disable make test-integration
#
# Export it rather than a `VAR=value make` prefix, which make does not pass to a
# recipe's environment, and which cmd.exe does not accept at all.
test-integration:
	go run ./cmd/testpg $(ARGS)

# Against a running service, over the network. Needs `make start` first — this
# talks to the deployed binary, not to an in-process server.
#
# Behind the `e2e` build tag, so the other targets never compile it.
test-e2e: export E2E_BASE_URL = http://localhost:3000
test-e2e:
	go test -count=1 --tags=e2e ./e2e/...

# The import rules the layout depends on are NOT here — they are a test
# (arch/arch_test.go), so they run with `make test` and need nothing installed.
#
# golangci-lint runs through `go run` at the version CI pins, so there is nothing
# to install and no drift between a laptop and the pipeline.
GOLANGCI_LINT_VERSION = v2.13.2

lint:
	go vet ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

# --- Modules ------------------------------------------------------------------
# Scaffold a module: routes, controller, service, dto, request, repository and
# both test files, wired the way modules/note is.
#
#   make new-module name=order
#
# It writes only under modules/; the four things it cannot do for you — the
# prisma schema, the model, the error codes, and the line in internal/httpapi —
# are printed as a checklist when it finishes.
new-module:
	@go run ./cmd/scaffold -name $(name)

# --- API reference ------------------------------------------------------------
# One parse of the registered routes writes two files, so neither can drift from
# them:
#
#   data/postman_collection.generated.json   gitignored — import it into Postman
#   apispec/openapi.generated.json           COMMITTED — go:embed serves it at /_docs
#
# The second is committed because go:embed needs it at compile time. Run this
# after adding or changing a route, and commit what changes.
#
# The generator lives in mine-core and is pinned by the tool directive in go.mod,
# so bumping core is what updates it. What this project names for itself — the
# collection name, the base URL, which route captures which token — is in
# postmangen.json.
postman:
	go tool postmangen

# One folder per top-level path segment, instead of one per module.
postman-flat:
	go tool postmangen -layout flat

# Fails when the committed OpenAPI document no longer matches the routes. CI runs
# it, so a route added without regenerating is a red pipeline rather than an
# endpoint quietly missing from /_docs — which nobody notices, because the page
# still loads and still looks complete.
api-check:
	go tool postmangen
	@git diff --exit-code -- apispec/openapi.generated.json \
		|| (echo 'apispec/openapi.generated.json is stale — run `make postman` and commit it' && exit 1)

# --- Agent instructions -------------------------------------------------------
# The skills in .claude/skills are written once, for every agent. Claude Code
# reads them where they are; Codex reads .codex/skills and has no setting that
# would point it elsewhere, and an agent with no skill mechanism reads the index
# in AGENTS.md. Both of those are generated from the skills themselves, so the
# prose is never copied and cannot drift.
#
# Run this after adding, renaming or re-describing a skill, and commit what
# changes. Editing the skill's body alone changes nothing here.
skills:
	go run ./cmd/skillsync

# Fails when the mirror or the index no longer matches .claude/skills. CI runs
# it beside api-check, so a skill added without regenerating is a red pipeline
# rather than a skill Codex silently never sees.
skills-check:
	go run ./cmd/skillsync -check
