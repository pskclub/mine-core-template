# mine-core-template

Service template on [mine-core v2](https://github.com/pskclub/mine-core)
(`github.com/pskclub/mine-core/v2`) — Echo, GORM, koanf, slog, Sentry.
Prisma owns the database schema; Go owns everything else.

## Start a service from this template

**With `gonew`** — it copies the template and rewrites every import to your
module path in one step:

```bash
go run golang.org/x/tools/cmd/gonew@latest github.com/pskclub/mine-core-template github.com/acme/orders
cd orders
```

**Or with GitHub's "Use this template" button**, then rename the module in the
new repository yourself (GNU sed shown; on macOS use `sed -i ''`):

```bash
go mod edit -module github.com/acme/orders
grep -rl 'github.com/pskclub/mine-core-template' --include='*.go' . | xargs sed -i 's#github.com/pskclub/mine-core-template#github.com/acme/orders#g'
```

Nothing else holds the module path: the import rules in `arch` and
`make new-module` read it from `go.mod`, so both keep working after a rename.
What is still named after the template is yours to change — `SERVICE` in
`.env.sample` and `test.env`, the collection in `postmangen.json` (then run
`make postman`), and `name` in `package.json`.

## Dev

```bash
cp .env.sample .env
make start          # api + postgres + redis
bun install
make migrate        # apply committed migrations
make logs
```

Running the binary on the host instead of in a container works with the same
`.env` — it points at `localhost`, and docker-compose overrides the hosts with
`APP_DB_CONNECTION_STRING` / `APP_CACHE_CONNECTION_STRING`:

```bash
make install        # fetch modules
make run            # API
make run-worker     # scheduled jobs
make dev            # API with live reload (make dev-worker / dev-all too)
```

### Live reload

`make dev` rebuilds and restarts the service on every save.
[air](https://github.com/air-verse/air) does the watching, and it is a tool
dependency in `go.mod` — same as postmangen — so `make dev` is `go tool air` and
there is nothing to install first. Bump it with
`go get -tool github.com/air-verse/air@<version>`, checking that the version's
own `go` directive is not ahead of this module's: `go get -tool` raises ours to
match, which would put every developer and CI on a new toolchain for the sake of
the dev loop.

[.air.toml](.air.toml) holds the settings and is written for all three
platforms: linux and macOS take the `[build]` table, Windows takes the
`[build.windows]` overrides, which exist because Windows will not execute a file
without an extension. So `make dev` needs no bash, no `chmod`, and no separate
Windows path — one recipe line, run from PowerShell, cmd or a POSIX shell.

The rest of the Makefile follows the same rule: on Windows make runs recipes
through `cmd.exe` unless an `sh.exe` happens to be on PATH, so no recipe carries a
`VAR=value command` prefix. Where a target needs an environment variable — the
role for `run-worker`, `E2E_BASE_URL` for `test-e2e` — it is a target-specific
`export`, which make applies itself and the child inherits.

Also in that file: a change to `.env` restarts the service (config is read at
startup, so nothing else would pick it up), test files do not trigger a rebuild,
and a broken build stops the old binary rather than leaving it to serve stale
code.

`make dev-worker` and `make dev-all` are the same loop in the other two roles.
The container in [dev.Dockerfile](dev.Dockerfile) reloads with the same config —
commented out in docker-compose by default, since running the binary on the host
is faster.

## Backing services

| Service | Image | Port | Used by |
|---|---|---|---|
| `db` | postgres:17-alpine | 5432 | `c.DB()`, prisma |
| `cache` | redis:7-alpine | 6379 | `c.Cache()` |

Both have healthchecks and `api` waits on them, so the first request never
races a database that is still starting. A connection is only opened when its
config is present — drop `CACHE_CONNECTION_STRING` and `c.Cache()` returns nil
instead of failing at boot.

## Schema — prisma owns it

Every table change starts in `prisma/schema/*.prisma`. The GORM models in
`models/` *describe* tables prisma created; nothing in Go migrates the database,
so there is one source of truth and no automigrate surprises.

```bash
make migrate-dev    # author a migration from the schema (host, prompts for a name)
make migrate        # apply committed migrations (container — same step in CI)
make seed           # run prisma/seed.ts
make generate       # regenerate @prisma/client after a schema change
make studio         # browse the data
```

`migrate-dev` runs on the host against the postgres published on `localhost`: it
needs a prompt and a shadow database. `migrate` runs `prisma migrate deploy` in
a container — it only applies what is already committed under
`prisma/schema/migrations/`, so it is safe to point at a shared database.
(Prisma keeps migrations beside the schema directory it was pointed at, which
`prisma.config.ts` sets to `prisma/schema`.)

Adding a table: edit the `.prisma` schema → `make migrate-dev` → add the
matching struct in `models/` → `make generate` if the seed touches it.

## Roles

One binary, one role per deployment. Set it as `ROLE=all` in `.env`, or
`APP_ROLE=all` in the environment (how docker-compose and Kubernetes set it) —
both resolve to the same config key.

> The `APP_` prefix belongs to OS environment variables only. `APP_ROLE=all`
> written *inside* `.env` binds to the key `app_role` and is silently ignored,
> leaving you on the default `api` role.

| `APP_ROLE` | Runs | Entry point | Replicas |
|---|---|---|---|
| *(unset)* / `api` | HTTP server | `cmd.APIRun` | many |
| `worker` | scheduler + jobs | `cmd.WorkerRun` | one |
| `all` | both, one process | `cmd.AllRun` | **one only** |

```bash
make run          # api
make run-worker   # jobs
make run-all      # both
```

`all` is the right choice for a small service or for local development: HTTP and
jobs share one `App`, so there is one set of connection pools rather than two,
and shutdown is ordered across both — requests drain, then jobs finish, then the
pools close.

> ⚠️ **Never scale `all` past one replica.** The default scheduler queue is
> in-memory and per-process, so every replica fires every cron tick — three
> replicas means the nightly report runs three times. To scale, run `api`
> replicas alongside a single `worker`.

## How it fits together

`cmd.Bootstrap` builds the **App** once: it loads config, opens every connection
that is configured, and hands out per-request/per-job contexts. Pools live for
the lifetime of the process, never per request.

A request flows: **route** (`modules/*/*.http.go`) → **handler** (binds +
validates a request struct) → **service** (business logic, takes any
`core.IContext`) → **store** (the module's own repository, which nothing outside
the module can reach).

Every layer returns `core.IError`, so a handler returns the error as-is and the
server renders `{code, message, fields}` with the right status. Wrapping a
repository error with `ctx.NewError(err, err)` keeps its status while reporting
5xx to Sentry at the point where the context still knows the user and the
request.

## Layout

The code is grouped by **feature**, not by layer. Everything a feature does —
routes, handlers, validation, business rules, queries, tests — is one directory,
so a change to it is one directory, and so a team can own it.

```
project
    - .env                  config (no APP_ prefix here; OS env uses APP_ and wins)
    - main.go               picks the role
    - cmd                   how the service is assembled and started
        - bootstrap.go      builds the App (config + connections)
        - modules.go        Modules — the one list of every module
        - api.go            NewAPI — the probes and this deployment's HTTP options
        - worker.go         newScheduler — the process's own jobs (not the modules')
        - run.go            core.Runner — ordered start and shutdown
        - scaffold          `make new-module`
    - testkit               test setup shared by every package
    - arch                  the import rules, as a test
    - modules
        - note              ← the worked example; read its README first
            - note.module.go      core.IModule — the module itself
            - note.http.go        its routes, and nothing else
            - handler             note.handler.go, note.request.go
            - service             note.service.go, note.dto.go
            - store               note.store.go — queries this module's table
        - user
            - user.api.go         what other modules may call; forwards to service
        - auth              routes AND its cron: auth.http.go + auth.jobs.go
        - home                    one file: not every module needs the layers
    - models                struct + column tags, shared by everyone
    - middlewares           what a route reaches for by name: AuthRequire(e)
    - repo                  query helpers that belong to no single table
    - emsgs                 this service's errors + validation messages, per module
    - consts / helpers / views
    - prisma                schema, migrations, seed
```

### The rules that keep it that way

1. **A module declares everything it attaches, in its own directory.**
   `<name>.module.go` implements
   [`core.IModule`](https://github.com/pskclub/mine-core/blob/master/v2/docs/modules.md): `Name` is all that
   is required, and `Routes`, `Jobs`, `Cron`, `Consumers`, `HealthChecks`,
   `Start`/`Stop` are optional interfaces a module adds as it grows — each in the
   file named for it (`<name>.http.go`, `<name>.jobs.go`, `<name>.mq.go`), so a
   file name stays true as the module grows. Registering a feature in two
   composition roots is how it ends up half-registered — and the missing half is
   the silent one: a cron nobody armed writes no log line, it simply never runs.
2. **A module owns its tables — and its routes, and its jobs.** Its repository is
   unexported; another module asks through its service interface. A shared
   repository package is the shortcut through which, a year in, everything
   queries everything and no column can be changed. The same goes for a cron job
   that deletes rows: how long a token's row is kept is an auth decision, so it
   is armed by [auth.jobs.go](./modules/auth/auth.jobs.go).
3. **No module imports another module,** and a module is entered through its
   top-level package only. It declares what it needs as an interface of its own
   and `cmd.Modules` supplies it. See
   [auth.deps.go](./modules/auth/service/auth.deps.go) for the pattern, and
   [user.api.go](./modules/user/user.api.go) for the surface a module offers.
4. **`models/` and `consts/` import nothing from this project,** and
   `middlewares/` imports no module. They sit below everything; a dependency
   back is what turns a shared package into a shared tangle.
5. **Routes are written in full, one middleware per line** —
   `e.GET("/notes/:id", c.Find, middlewares.AuthRequire(e))`. A path in a log is
   greppable, and whether a route is protected is answerable on the line
   itself.

Rules 2–4 are enforced by [arch](./arch/arch_test.go), which runs under
`make test`. Rule 5 is enforced by each module's `requiresAuthentication` test.

One module list serves every role. `core.RunModules` attaches it to whatever the
role actually has — cron only where there is a scheduler, routes only where there
is a server — and the boot line names what it had nowhere to put:

```
modules mounted  modules=[home auth user note exchange] routes=19 jobs=0 cron=0 queues=0 skipped=[cron]
```

`skipped=[cron]` is normal in an `api` replica. In a process that was meant to
arm those schedules it is the misconfiguration, and nothing else reports it.

### Adding a module

```bash
make new-module name=order
```

It writes the eight files and prints the four things it cannot decide for you:
the prisma schema, the model, the `allModels()` line, and the one line in
`cmd.Modules`. Start from [modules/note](./modules/note/README.md) if you
want to see the shape first.

## Errors

Errors this service defines live in [emsgs](./emsgs/), **one file per module** —
`auth.emsgs.go`, `user.emsgs.go` — so an error sits next to the others from the
same part of the service. Declared once and reused: a code is what clients branch
on, so a typo in a repeated string literal is a silent break.

```go
return emsgs.InvalidCredentials      // 401 INVALID_CREDENTIALS
```

Statuses come from `net/http` rather than a literal: `http.StatusUnauthorized`
says what 401 means without the reader having to recall the number.

The framework's own errors (`NotFound`, `BadRequest`, `DBError` …) come from
mine-core's `errmsgs` — use those directly; `emsgs` is only for what is specific
to this service.

Validation codes raised with `v.Must` have no message of their own, so `emsgs`
registers one per code in `init`. Keeping the code and its wording together
avoids the message living in whichever request happened to need it first:

```go
const CodeDuplicateInRequest = "DUPLICATE_IN_REQUEST"

func init() {
    valid.SetMessage(CodeDuplicateInRequest, "The {field} field is repeated in this request")
}
```

Because using a code means importing `emsgs`, its `init` always runs — a code
cannot be raised without its message being registered.

## Logging

The logger comes off the context, where it is already bound to the unit of work:
under HTTP it carries the request id, under the scheduler the job name, run id
and attempt. Nothing holds it and nothing passes it.

```go
s.ctx.Log().Info("user created", "user_id", user.ID)
```

```json
{"level":"INFO","msg":"user created","request_id":"raBcQ…","user_id":"3830…"}
{"level":"INFO","msg":"POST /auth/register 201 41ms","status":201,"request_id":"raBcQ…"}
```

The request id is what joins them, which is why a service never needs a logger of
its own. Key-value pairs, never `fmt.Sprintf` — the output is JSON, and a field
can be searched and aggregated where a sentence cannot. `ctx.Log().With(...)`
exists for the rare case that several lines in one function share a field.

### What is already logged for you

Adding a line that repeats one of these makes one event look like two:

| Already recorded | By |
|---|---|
| method, path, status, latency, request id, error code | the request middleware, at Info/Warn/Error by status |
| every 5xx, with the request's user and scope | Sentry, from `ctx.NewError` |
| failed and slow SQL — every statement at `LOG_LEVEL=debug` | the GORM logger |
| a job's start, failure and retry | the job runner |
| panics | the recover middleware, at fatal |

So a handler writes nothing, a store writes nothing, and **an error that is
returned is not also logged**.

### What is worth a line, and at which level

- **Info** — a write that changed data (`user created`, `note deleted`,
  `signed in`), and a scheduled run reporting what it did *every* time, count
  zero included: a job that only speaks when it acts cannot be told apart from
  one that has stopped.
- **Warn** — the service working as designed while refusing the caller:
  `sign-in failed` with the reason, a business rule rejecting a valid request,
  a token naming an account that no longer exists. These are the lines you alert
  on by rate.
- **Error** — only what nobody else will report. In this repository that is
  exactly two places: a route registered without its middleware, and
  `RegisterAuth` never called.
- **Debug** — detail for the ten minutes someone is investigating: what a list
  query actually filtered by, which of three reasons a token was rejected.
  Anything that runs on every request belongs here.

The clearest case is [Login](./modules/auth/service/auth.service.go): the
response says only `INVALID_CREDENTIALS`, because telling a client apart "no such
account" from "wrong password" hands them an account enumerator. The log records
which it was — it never leaves the service, and whoever is looking at a burst of
failures needs exactly that.

### What must never reach a log

Tokens and their digests, password hashes, and anything else that would be a
credential in a store that more people can read than the database. Personal data
by id, not by value: `user_id`, not the email address; `note_id`, not the note.

## Adding a config key

Add one field to `core.ENVConfig` upstream, or read it ad hoc:

```go
c.ENV().String("some_key")   // APP_SOME_KEY, or SOME_KEY in .env
```

## Calling another service

[modules/exchange](./modules/exchange/service/exchange.service.go) is the worked
example: a module whose data lives somewhere else. No table, no prisma schema, no
`store/` — the repository is an HTTP call.

Every call out goes through `core.Requester(ctx)`, never `http.Get` and never a
client built for the occasion:

```go
ctx, cancel := context.WithTimeout(s.ctx, upstreamTimeout)
defer cancel()

r := core.Requester(ctx)

var out ratesResponse
var fail upstreamError

_, err := r.Send(
    r.R().
        SetQueryParams(map[string]string{"base": base, "symbols": quote}).
        SetResult(&out).   // body on success
        SetError(&fail),   // body on failure
    http.MethodGet, baseURL+"/latest",
)
```

What that buys, and what a hand-rolled client throws away:

- **the caller's deadline.** A request the client has already abandoned stops the
  call it was waiting on, instead of holding a connection for the upstream's
  timeout. `core.Requester` takes any context — including one with a tighter
  deadline of our own, which is what bounds a single call without touching the
  shared client.
- **the caller's trace.** Every call leaves a breadcrumb on the request that made
  it and propagates the trace headers, so an issue in Sentry shows what we asked
  and what came back with nobody logging it by hand.
- **one client, one connection pool,** configured once at startup.

### Their failure is not your clients' failure

`Send` already returns a `core.IError` carrying the upstream's status and code.
That is exactly what must not reach a client of ours: a caller branching on the
provider's codes breaks the day we change provider, and it leaks who we buy from.
So every outcome maps to an error this service owns, and the original is kept as
the cause for the log and for Sentry:

| Upstream | We answer | Why |
|---|---|---|
| 404 / `unknown_currency` | `400 UNKNOWN_CURRENCY` | the request was well-formed; the client can ask for another pair |
| 429, 5xx, timeout, refused | `503 EXCHANGE_UNAVAILABLE` | one answer, because the client's move is the same in all of them |
| 200 without the pair in it | `400 UNKNOWN_CURRENCY` | reading the zero value out of the map is how a missing quote becomes a rate of 0.00 |
| no provider configured | `500 EXCHANGE_NOT_CONFIGURED` | nothing gets better by waiting; a 503 would hide an unfinished deployment |

`SetError` is what makes the first two rows possible: it decodes their error body
into a type of ours, so the service can branch on the fields a status and a code
cannot carry — `retryable`, `retry_after`.

## Testing it

The provider is faked with `httptest.Server`, not by mocking `IRequester`: both
ends are real and only the far end is ours, so the query parameters we send, the
JSON we decode and the statuses we map are all covered — which is where this kind
of code actually breaks.

```go
ctx := testkit.Context(t, coretest.WithEnv(map[string]string{
    "exchange_base_url": upstream.URL,
}))
```

See [exchange.service_test.go](./modules/exchange/service/exchange.service_test.go)
for the failure cases, and [exchange.module_test.go](./modules/exchange/exchange.module_test.go)
for the same thing through the real server.

> The endpoint is off until `EXCHANGE_BASE_URL` is set — it answers
> `EXCHANGE_NOT_CONFIGURED` rather than quietly calling a third party nobody
> chose. The cache is optional too: with no redis configured `ctx.Cache()` is nil
> and the service still works, only slower.

## Endpoints

| Method | Path | Auth | Notes |
|---|---|---|---|
| `GET` | `/` | — | root |
| `GET` | `/healthz` | — | liveness — touches no dependency, so a database hiccup never restarts the pod |
| `GET` | `/readyz` | — | readiness — pings every pool; 503 takes the instance out of the load balancer |
| `POST` | `/auth/register` | — | creates an account |
| `POST` | `/auth/login` | — | returns a token |
| `POST` | `/auth/logout` | token | revokes the token used |
| `GET` | `/auth/me` | token | the signed-in user |
| `GET` `POST` | `/users` | token | list (paginated) / create |
| `POST` | `/users/bulk` | token | create many — all of them or none |
| `GET` `PUT` `DELETE` | `/users/:id` | token | |
| `GET` `POST` | `/notes` | token | the signed-in user's notes |
| `GET` `PUT` `DELETE` | `/notes/:id` | token | |
| `GET` | `/exchange-rates/:base?quote=` | token | calls a third-party API — see below |

See [api.http](./api.http) for runnable examples of the whole flow.

### API reference at `/_docs`

The running service serves its own reference: every endpoint, with a button that
sends it, at **`/_docs`** — and the raw document at `/_docs/openapi`.

It is the same parse that writes the Postman collection, embedded into the
binary, so it always describes exactly the code the process was built from. The
page sends its requests back to whatever host served it, so it works the same
from `localhost`, from staging, and through a `kubectl port-forward`, with
nothing to configure.

> `apispec/openapi.generated.json` is **committed**, unlike the Postman
> collection: `go:embed` needs it at compile time. `make api-check` fails when it
> no longer matches the routes, and CI runs it — a route added without
> regenerating is a red pipeline rather than an endpoint quietly missing from a
> page that still looks complete.

### Postman collection

`make postman` reads the registered routes and writes both files, ready to
import:

```bash
make postman              # folders grouped by module
make postman-flat         # one folder per top-level path segment
make api-check            # fails if the committed OpenAPI document is stale
```

Nothing is maintained by hand — the generator parses the source, so the
collection cannot drift from the routes. It carries over the group prefix and the
middleware applied to a group, the request body built from the payload struct,
the query parameters with the rules they must satisfy, and an example response
for the success the handler writes plus the `400` and `401` the framework
answers with.

The generator itself lives in mine-core, pinned by the `tool` directive in
`go.mod` — most of what it reads is the framework's own contract, so bumping core
is what keeps it current. What this project names for itself is in
[postmangen.json](./postmangen.json): the collection name and id, the base URL,
and which sign-in route stores its token in which variable. See
[the core docs](https://github.com/pskclub/mine-core/blob/master/v2/docs/postman.md)
for every option.

Sample values come from each request's `Valid` method, so a generated body is one
the server accepts: `Email()` produces an address, `In("ADMIN", "MEMBER")`
produces `ADMIN`, `Length(8, 72)` produces something long enough. Signing in
stores the token in a collection variable, so every protected request
authenticates itself from then on.

The file is generated, not committed. Regenerate it after adding a route.

## Authentication

Opaque bearer tokens, verified against the database. No JWT, no signing secret.

```
POST /auth/login  ->  { "id": "…", "email": "…", "full_name": "…",
                        "token": "9f3c…", "expires_in": 86400 }

Authorization: Bearer 9f3c…
```

The user is flat, alongside the token, rather than nested under a `user` key —
the model is embedded in `AuthResult`, and `encoding/json` promotes an anonymous
struct's fields.

**Only the SHA-256 digest of a token is stored** ([auth.service.go](./modules/auth/auth.service.go)),
never the token itself — a database that leaks exposes no usable credentials, the
same reason passwords are hashed. The plaintext is shown once, at sign-in.

Plain SHA-256 is right for the token and bcrypt is not: a token is 256 bits of
`crypto/rand`, so there is nothing to brute-force, and the lookup runs on every
request — a deliberately slow hash would be a denial of service on ourselves.
Passwords are the opposite case and use bcrypt.

Because a token is a row, **revocation is a DELETE**: `POST /auth/logout` stops it
working immediately, which a self-contained token cannot do without a blocklist
consulted on every request. Tokens expire after 24h (`auth.TokenTTL`).

Sign-in answers identically for a wrong password and an unknown address, so the
endpoint cannot be used to discover which addresses are registered.

A protected route says so on its own line
([user.http.go](./modules/user/user.http.go)):

```go
e.GET("/users/:id", c.Find, middlewares.AuthRequire(e))
```

[middlewares](./middlewares/auth.go) imports no module, and cannot: every
module's routes import it. The token lookup is handed to it once at startup by
[cmd.Modules](./cmd/modules.go), which is the only place that
knows auth issues the tokens and the user module owns the accounts.

The cost of writing the middleware per route instead of once on a group is that
a route added without it is open. Each module's `requiresAuthentication` test names
every route and asserts each is closed to an anonymous caller, which is what
turns that into a test failure rather than an incident.

`c.GetUser()` returns the caller inside any protected handler. For roles, core
also provides `a.RequireRole("admin")` and `a.Optional()`.

## Inspecting a running service

Two read-only panels, both from mine-core, both off unless asked for:

| | | |
|---|---|---|
| `/_docs` | API reference | every endpoint, with a button that sends it |
| `/_dev` | inspector | capabilities and pool stats, routes and who serves them, config with secrets withheld, jobs and their runs, **modules**, health |

The **modules** tab is the one worth knowing about. It reports what each module
actually attached *to this process* — which is the question a module system
otherwise makes harder, not easier: a `worker` replica that never mounted a
module's routes and an `api` replica that never armed its cron look perfectly
healthy from every other angle. A module listed as *registered nothing in this
process* is the finding.

**When they are on** ([cmd/inspect.go](./cmd/inspect.go)):

| | |
|---|---|
| `ENV=dev` | on, unprotected — a laptop |
| `DEVTOOLS_PASSWORD` / `APIDOCS_PASSWORD` set | on anywhere, behind a browser login (user defaults to `devtools` / `apidocs`) |
| neither | off, and the routes do not exist |

That gate is in this project, not in the framework: mine-core *refuses to mount*
outside dev without a guard, which would stop a production boot. Asking the same
question first turns "refuse to start" into "do not mount", so turning the panels
on in staging is a deployment change and not a code change.

Writing — triggering a job, cancelling a run — stays off outside dev even when
the panel is open behind a password. Reading what a process is doing and re-running
last night's billing job are not the same permission.

## Testing

Two backends, one suite — the same tests run against either:

```bash
make test              # sqlite in memory: no dependencies, ~0.5s
make test-integration  # real postgres, real schema — needs nothing installed
```

`make test` is the one to run constantly. It gives every test its own
`:memory:` database, so nothing needs cleaning up and tests cannot interfere.

`make test-integration` is the one that must pass before you push. It gives each
test a **temporary schema** (dropped afterwards) built by applying
`prisma/schema/migrations` — the same SQL production runs.

That difference matters: sqlite has no partial indexes, no UUID column type and
no `ILIKE`, and `AutoMigrate` invents a schema from the Go structs rather than
reading the prisma one. A suite that only ever sees sqlite will happily pass on
constraints postgres rejects.

### The postgres comes from the test run itself

`make test-integration` needs no docker, no daemon and nothing installed.
[cmd/testpg](./cmd/testpg/) fetches a real postgres 17 binary (once, cached under
`$HOME/.embedded-postgres-go`), runs it on a private port for the length of one
`go test`, hands the suite its DSN through `TEST_DATABASE_URL`, and deletes the
cluster when the run ends. The suite itself needed no change to use it: testkit
already switches to postgres when that variable is set, and that is the only
thing the tool hands over.

It is [github.com/fergusstrange/embedded-postgres](https://github.com/fergusstrange/embedded-postgres)
underneath, configured so the cluster is identical on every machine — `UTF8`/`C`
rather than whatever the host locale is (on Windows that is WIN1252, under which
every non-latin1 literal in a test fails to bind before an assertion runs),
`fsync=off` on a cluster that is about to be deleted, and `max_connections=200`
because parallel packages each take two.

`ARGS` passes flags and packages through to `go test`:

```bash
make test-integration ARGS='-run TestNoteAPI_crud ./modules/note/...'
make test-integration ARGS='-pglog'    # and show the postgres server log
```

### Where `TEST_DATABASE_URL` goes

It is read straight from the process environment — **not** from `.env`, and with
no `APP_` prefix. Putting it in `.env` does nothing; the suite silently stays on
sqlite.

A `TEST_DATABASE_URL` that is already exported **wins, and testpg starts
nothing** — that is how you point the same target at the postgres from
`make start`, or at a shared database:

```bash
TEST_DATABASE_URL=postgres://my_user:my_password@localhost:5432/my_database?sslmode=disable make test-integration
```

| Where | How |
|---|---|
| `make test-integration` | set by `cmd/testpg` for the postgres it started |
| one-off shell | `TEST_DATABASE_URL="postgres://…" go test ./...` |
| CI | `.github/workflows/ci.yml` → the `test (postgres)` job |
| IDE | VS Code `go.testEnvVars`, GoLand run-configuration env |

VS Code, in `.vscode/settings.json` (gitignored, so it stays yours):

```json
{
  "go.testEnvVars": {
    "TEST_DATABASE_URL": "postgres://my_user:my_password@localhost:5432/my_database?sslmode=disable"
  }
}
```

Both CI jobs run on every push: `test` on sqlite for fast feedback, and
`test:integration` against a postgres service on the real schema.

Both paths share one harness — [testkit](./testkit/testkit.go).
Adding a model means adding it to `allModels()` there, and nowhere else.

Services take a `core.IContext`, so the same code runs in a handler, a job or a
test with no test-only interfaces:

```go
ctx := testkit.Context(t)
u, err := user.NewUserService(ctx).Create(payload)
```

For the endpoints, `testkit.Serve` starts the **whole** service on a real
listener, from the same `cmd.Modules` set `cmd` assembles — so a module
registered without its guard, or with a dependency left unsatisfied, fails in the
test rather than in production:

```go
anon, app := testkit.Serve(t)
c := testkit.SignIn(t, anon)
```

> `APP_ENV`/`test.env` are not used by these tests — the harness builds config
> from environment variables so results never depend on files in your checkout.

## Coding agents

The conventions above are also written for the agents that work in this
repository, once, in one place — [.claude/skills/](./.claude/skills/), one skill
per subject. A `SKILL.md` is agent-neutral (markdown behind a `name` and a
`description`), but every agent looks for skills somewhere else, so the rest is
generated:

| | |
|---|---|
| [.claude/skills/](./.claude/skills/) | the skills themselves — the only copy anyone edits |
| `.codex/skills/` | one stub per skill, pointing back; Codex's directory, and it has no setting that could point elsewhere |
| [AGENTS.md](./AGENTS.md) | the [agents.md](https://agents.md) entry point: defers to [CLAUDE.md](./CLAUDE.md) for the conventions, and carries the skills index for agents with no skill mechanism |

`make skills` writes the last two from the first; `make skills-check` fails if
they have drifted and runs in CI beside `make api-check`. So a skill is written
and reviewed once, and no agent reads a stale copy of it — the same reason
`apispec/openapi.generated.json` is generated rather than maintained.

## Docs

The framework manual is at **[mine-core/v2/docs](https://github.com/pskclub/mine-core/tree/master/v2/docs)**
(source: `mine-core/v2/docs/`).

The pages that describe *this* layout — written from this repository, so they
explain the same decisions in more detail than a README can:

| | |
|---|---|
| [Project Structure](https://github.com/pskclub/mine-core/blob/master/v2/docs/structure.md) | modules, the layers inside one, and the import rules `arch` enforces |
| [Lifecycle & Roles](https://github.com/pskclub/mine-core/blob/master/v2/docs/lifecycle.md) | `cmd.Bootstrap`, the api/worker/all split, ordered shutdown |
| [Middleware & Routing](https://github.com/pskclub/mine-core/blob/master/v2/docs/middleware.md) | the guard that imports no module, one middleware per route |
| [Service Errors](https://github.com/pskclub/mine-core/blob/master/v2/docs/service-errors.md) | `emsgs`, validation code vs business rule |
| [Logging Practices](https://github.com/pskclub/mine-core/blob/master/v2/docs/logging-practices.md) | what is already logged, and at which level to add to it |
| [Schema & Migrations](https://github.com/pskclub/mine-core/blob/master/v2/docs/migrations.md) | why prisma owns the schema and Go does not |
| [Deployment](https://github.com/pskclub/mine-core/blob/master/v2/docs/deployment.md) | image, probes, replicas per role, grace periods |

Reference for the rest: [getting-started](https://github.com/pskclub/mine-core/blob/master/v2/docs/getting-started.md),
[env](https://github.com/pskclub/mine-core/blob/master/v2/docs/env.md),
[http](https://github.com/pskclub/mine-core/blob/master/v2/docs/http.md),
[validation](https://github.com/pskclub/mine-core/blob/master/v2/docs/validation.md),
[repository](https://github.com/pskclub/mine-core/blob/master/v2/docs/repository.md),
[jobs](https://github.com/pskclub/mine-core/blob/master/v2/docs/jobs.md),
[testing](https://github.com/pskclub/mine-core/blob/master/v2/docs/testing.md),
[sentry](https://github.com/pskclub/mine-core/blob/master/v2/docs/sentry.md).
