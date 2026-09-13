---
name: mine-core-review
description: Review a change against this mine-core service's conventions before it is pushed or merged — layering, import rules, errors, ownership, validation placement, logging, tests, schema and config. Use when reviewing a diff or a merge request, or as a self-check after finishing a feature.
---

# Reviewing a change

Run the checks first, then read the diff against the list. Findings should name
the rule and the file — every item below is a decision this codebase already
made, documented in [README.md](../../../README.md) and
[CLAUDE.md](../../../CLAUDE.md).

```bash
gofmt -l . && make lint && make test
make test-integration      # if the schema, a query, or anything SQL-shaped changed
make skills-check          # if a skill was added, renamed or re-described
```

## Layering

- [ ] Routes only in `Module.Routes` in `modules/<name>/<name>.module.go`. Full paths, no groups, one
      middleware per line.
- [ ] Handler does four things: bind, convert, call, render. **No decision, no
      query, no error message built there.**
- [ ] Service takes `core.IContext` — not `IHTTPContext`, not a concrete type —
      so a job can call it.
- [ ] Store is the only package touching its module's tables, and it is silent.
- [ ] A file's name matches its directory (`order.handler.go` in `handler/`).

## Import rules (arch/arch_test.go fails on these — but catch them in review)

- [ ] No module imports another module. A dependency is **the consumer's own
      narrow interface** in `service/<name>.deps.go`, supplied by `cmd.Modules`.
- [ ] Nothing outside `modules/x` imports `modules/x/service` or `/store`. What
      is offered goes in `<name>.api.go` as aliases plus one-line forwards.
- [ ] `models/`, `consts/` import nothing local; `repo/`, `emsgs/`, `helpers/`,
      `middlewares/` import no module and no `cmd`.
- [ ] `testkit` only from `_test` files.
- [ ] A new `<name>.api.go` line is a deliberate widening — is it needed?

## Errors

- [ ] Every cross-package return is `core.IError`.
- [ ] New codes are sentinels in `emsgs/<module>.emsgs.go`; statuses from
      `net/http`, never a literal; no framework error redefined.
- [ ] Repository failures wrapped with `ctx.NewError(err, err)`.
- [ ] **No upstream code or message reaches our clients** — every outcome mapped.
- [ ] A `v.Must` code has its `valid.SetMessage` in the same file's `init`.

## Authorization and ownership

- [ ] Every new route carries its middleware, **and is named in
      `Test<Name>API_requiresAuthentication`.**
- [ ] Ownership is an explicit first argument, applied as a scope on **every**
      statement — including the write after the read.
- [ ] "Not yours" answers **404**, never 403.
- [ ] The owner comes from the token, never from the request body.

## Validation

- [ ] Payload-only rules in `Valid`; rules that need rows in the service,
      returning an `emsgs` sentinel (usually 409).
- [ ] Optional fields are not `Required()` — a missing bool means false.
- [ ] Pointer fields, so absent and empty differ.

## Data

- [ ] Schema change went through prisma **and** `models/` **and** `allModels()`
      in `testkit.go`.
- [ ] No prisma relation or GORM association across module boundaries.
- [ ] `Updates` with a **map** for any column that can hold `false`/`0`/`""`.
- [ ] `repo.DefaultOrder(opts, ...)`, not `.Order()`, alongside `Pagination`.
- [ ] `GetPageOptionsWithAllowed`, not `GetPageOptions`, for anything sortable.
- [ ] Index matches the real access pattern, not just the foreign key.

## Logging

- [ ] `ctx.Log()` at the point of use, never a struct field. Key–value pairs, no
      `fmt.Sprintf`.
- [ ] Nothing that repeats the request line, the SQL logger, the job runner or
      Sentry. **An error that is returned is not also logged.**
- [ ] Info = a write that changed data or a scheduled run (count zero included);
      Warn = correctly refusing a caller; Error = only what nobody else reports.
- [ ] **No token, digest, password hash, email or user content by value** — ids
      only.

## Outbound calls

- [ ] `core.Requester(ctx)` — never `http.Get` or a per-call client.
- [ ] A per-call timeout, `SetResult`/`SetError`, and a `nil`/`ok` check before
      reading the decoded body.

## Tests

- [ ] The `requiresAuthentication` test names every route, including new ones.
- [ ] A cross-account isolation test for anything owned.
- [ ] Validation asserted by **field code**, never by message.
- [ ] Upstreams faked with `httptest`, not by mocking `IRequester`.
- [ ] `testkit.SignIn`, not a forged token or a hand-inserted session.
- [ ] `-race` still clean.

## Config, jobs and wiring

- [ ] New keys documented in `.env.sample`; no `APP_` prefix inside `.env`.
- [ ] A missing required key fails loudly rather than defaulting silently.
- [ ] A new job is owned by its module, prefixed with the module name, logs one
      line per run, and takes its clock as a parameter.
- [ ] A new module has exactly one new line in `cmd.Modules`, and everything it
      attaches (routes, cron, checks) is declared in its own `<name>.module.go`.
- [ ] `make postman` re-run if routes changed.

## Docs

- [ ] A **new pattern** (not a new instance of an existing one) is explained in
      the README or the module's own README. This codebase documents *why*, not
      *what* — match that voice, and do not restate what the code says.
