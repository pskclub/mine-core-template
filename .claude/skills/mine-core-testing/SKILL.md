---
name: mine-core-testing
description: Write and run tests for this mine-core service — testkit.Context/Serve/SignIn, the sqlite vs postgres vs e2e tiers, module HTTP tests, service tests, faking an upstream, and what every module's test file must cover. Use when adding tests, when a test fails, or before pushing a change.
---

# Testing

```bash
make test                 # unit + module + arch, sqlite in memory — run on every save
make test-integration     # the same suite on a real postgres it starts itself — run before pushing
make test-e2e             # against a *running* service, build tag `e2e`
go test ./modules/note/... -run TestNoteAPI_crud -v -count=1
```

Three tiers, and each answers something the others cannot:

- **sqlite** (`make test`) — every test gets its own `:memory:` database. Fast
  enough to run constantly.
- **postgres** (`make test-integration`) — every test gets a temporary schema
  built from `prisma/schema/migrations`, the SQL production runs, on a postgres
  [cmd/testpg](../../../cmd/testpg/) stands up itself: it fetches a real
  postgres 17 (once, cached under `$HOME/.embedded-postgres-go`), runs it on a
  private port for one `go test`, and deletes the cluster. Nothing installed, no
  docker. **This is the one to run before pushing**: sqlite has no partial
  indexes, no UUID type and no `ILIKE`, and its binder accepts
  `SELECT count(*) … ORDER BY`, which is the shape `core.Paginate` builds and
  postgres rejects.
  `ARGS` passes flags and packages through:
  `make test-integration ARGS='-run TestNoteAPI ./modules/note/...'`, and
  `ARGS='-pglog'` shows the postgres server log.
  A `TEST_DATABASE_URL` already exported wins and testpg starts nothing — that
  is how you point the same target at the docker-compose postgres, a shared
  database, or CI's service container.
- **e2e** (`make test-e2e`) — a compiled binary over the network, with the config
  and migrations it applied itself. Keep [e2e/](../../../e2e/) small: everything
  cheaper to test belongs in the package that owns the code.

The first two go through [testkit/testkit.go](../../../testkit/testkit.go),
which is imported **only from `_test` files** (arch enforces it), and switch on
the same thing — `TEST_DATABASE_URL` — so nothing in the suite knows which
postgres it is running against, or who started it.

## The three entry points

```go
ctx := testkit.Context(t)          // isolated db — service- and store-level tests
anon, app := testkit.Serve(t)      // the whole service on a real listener, from cmd.Modules
c := testkit.SignIn(t, anon)       // a client carrying a real token
```

`Serve` stands up **every** module from the same `cmd.Modules` production
uses — so a module registered with a dependency left unsatisfied fails here
rather than in production. A test that registered routes by hand would pass with
a mistake in the composition root still in place.

Extra options pass straight through:

```go
ctx := testkit.Context(t, coretest.WithEnv(map[string]string{"exchange_base_url": url}))
```

## Fixtures

`testkit.SignIn` registers and logs in through the real endpoints — never forge
a token, or the test keeps passing after the real path breaks. `testkit.SeedUsers`
writes rows straight through the database, because a test arranging its starting
state should not depend on the code it is about to exercise (seeded users cannot
sign in — their password is a literal, not a hash).

A second account: copy `signInAs` from
[note.module_test.go](../../../modules/note/note.module_test.go).

## What a module's tests must cover

`<name>.module_test.go` (package `<name>_test`):

1. **`Test<Name>API_requiresAuthentication`** — names **every** route and asserts
   an anonymous caller gets 401. This is the safety net for per-route middleware;
   **every new route goes in it.**
2. `Test<Name>API_crud` — the happy path, asserting the response shape.
3. **The isolation test** — a second account gets 404 (never 403) on list, read,
   update and delete of the first account's row, and the row is untouched
   afterwards.
4. `Test<Name>API_validation` — assert **field codes**, not messages:
   `FieldCodes()["title"] == "REQUIRED"`, `Error().Fields["id"].In == "path"`.
5. Ordering, search and paging if the module lists.

`service/<name>.service_test.go` covers the rules that need no HTTP — limits,
state transitions, error mapping — on `testkit.Context(t)`.

## The client

```go
c.Post("/notes", map[string]any{...}).RequireStatus(http.StatusCreated).Map()
c.Get("/notes?limit=10&page=1").RequireStatus(200).Map()["total"]
c.Put(path, `{"raw":"json"}`)      // a string body is sent verbatim
res.Error().Code                   // {code, message, fields} decoded
res.FieldCodes()                   // field -> code
res.JSON(&dest)
```

Assertions on a `core.IError` directly: `coretest.RequireCode`,
`RequireStatus`, `RequireNoError`, `RequireIs`, `FieldCodes`, `Fields`.

## Faking an upstream

Point config at an `httptest.Server` — **do not mock `IRequester`**. Both ends
stay real and the test then covers the query parameters sent, the JSON decoded
and the statuses mapped, which is where this kind of code actually breaks:

```go
srv := httptest.NewServer(http.HandlerFunc(handler))
t.Cleanup(srv.Close)

ctx := testkit.Context(t, coretest.WithEnv(map[string]string{"exchange_base_url": srv.URL}))
```

Worked example, including the malformed-200 case:
[exchange.service_test.go](../../../modules/exchange/service/exchange.service_test.go).

## Conventions

- `require` for preconditions (stops the test), `assert` for value checks.
- `t.Run` subtests for the cases of one behaviour; the parent name reads as the
  rule being proved.
- Never assert on wording — assert codes, statuses and ids.
- `TEST_DATABASE_URL` is read from the **process environment only**. Putting it
  in `.env` does nothing and the suite silently stays on sqlite.
- `go test -race ./...` is clean — keep it that way; the framework has no global
  state and the auth guard is keyed per-App so tests can run parallel.

Long form: `$CORE/docs/testing.md` and its `testing-*.md` siblings.
