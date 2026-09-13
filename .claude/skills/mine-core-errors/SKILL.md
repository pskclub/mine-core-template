---
name: mine-core-errors
description: Declare and return errors in this mine-core service — core.IError, the emsgs sentinels, framework errmsgs, wrapping with ctx.NewError, choosing a status, and mapping an upstream's failure to one of ours. Use when adding an error code, deciding a status, handling a repository or upstream failure, or when an error reaches a client in the wrong shape.
---

# Errors

Every layer returns `core.IError`. A handler returns it as-is; the server renders
`{code, message, fields}` with the error's own status. **No layer ever returns a
bare `error` across a package boundary, and no layer builds a message at the
call site.**

## Where an error comes from

| Kind | Source | Example |
|---|---|---|
| Framework | mine-core's `errmsgs` — use directly | `errmsgs.NotFound`, `BadRequest`, `Unauthorized`, `Forbidden`, `DBError`, `InternalServerError` |
| This service's own | [emsgs/](../../../emsgs/), one file per module | `emsgs.NoteLimitReached`, `emsgs.InvalidCredentials` |
| Field validation | `valid` — raised by `Valid`, rendered into `fields` | `REQUIRED`, `INVALID_UUID` |

Do not redefine a framework error in `emsgs`. Do not invent a code inline.

## Declaring one

```go
// emsgs/note.emsgs.go
var NoteLimitReached = core.New(
    http.StatusConflict,
    "NOTE_LIMIT_REACHED",
    "you have reached the maximum number of notes",
)
```

- Statuses come from `net/http`, **never a literal**.
- Errors are **sentinels** — compare with `errors.Is`, which matches by code
  through any wrapping.
- Every builder returns a **copy**, so enriching one never disturbs the shared
  value: `emsgs.UnknownCurrency.WithFields(map[string]any{"quote": quote})`,
  `errmsgs.NotFound.WithMessage("...")`.
- A validation code raised with `v.Must` also needs `valid.SetMessage` in that
  file's `init` — see `mine-core-validation`.

## Choosing a status

- **400** the payload is wrong → almost always a `Valid` rule, not a sentinel.
- **401** no valid caller. **403** a valid caller in the wrong role.
- **404** not there — **and also "not yours"**. A 403 on someone else's row
  confirms the id is real, which the caller has no right to know.
- **409** the request is well-formed but the state refuses it (a limit reached,
  a duplicate). This is the usual status for a service-level business rule.
- **422** rarely — prefer 400 with fields.
- **5xx** only what nobody outside can fix. A misconfiguration is one.

## Wrapping

```go
count, err := store.Note(s.ctx).Scopes(store.OwnedBy(ownerID)).Count()
if err != nil {
    return nil, s.ctx.NewError(err, err)     // repository error: keep its status
}
```

`ctx.NewError(cause, errorType, args...)` is what preserves the status **and**
reports 5xx to Sentry with this request's user, tags and breadcrumbs. The
repository already returns a typed `IError` (a missing row is `NotFound`, a
driver failure is `DBError`), so `s.ctx.NewError(err, err)` is the idiom: keep
what it decided, add the reporting.

To *replace* the outcome, pass the error you want as the second argument:

```go
return nil, s.ctx.NewError(err, emsgs.ExchangeUnavailable)   // cause kept for Sentry
```

**An error that is returned is not also logged.** The framework logs every 5xx
through `ctx.NewError` and every failed job run. A log line beside a `return` is
the same failure written twice.

## Mapping an upstream's failure to ours

An upstream code must never reach our clients — a caller branching on the
provider's codes breaks when we change provider. Map every outcome:

```go
switch {
case fail.IsUnknownCurrency() || err.GetStatus() == http.StatusNotFound:
    log.Warn("unknown currency pair")
    return emsgs.UnknownCurrency.WithFields(...)          // their 404 is our 400

case err.GetStatus() == http.StatusTooManyRequests:
    log.Warn("rate provider is throttling us")
    return s.ctx.NewError(err, emsgs.ExchangeUnavailable) // our quota, our 503

default:
    return s.ctx.NewError(err, emsgs.ExchangeUnavailable) // their 5xx / timeout
}
```

Worked example with the reasoning:
[exchange.service.go](../../../modules/exchange/service/exchange.service.go).

## Inspecting one

`err.GetStatus()`, `err.GetCode()`, `err.GetMessage()`, `err.OriginalError()`,
`errors.Is(err, emsgs.NoteLimitReached)`.

## Testing

Assert the code and status, never the wording:

```go
res := c.Get("/notes/" + id).RequireStatus(http.StatusNotFound)
assert.Equal(t, "NOT_FOUND", res.Error().Code)

// service level
coretest.RequireCode(t, err, "NOTE_LIMIT_REACHED")
coretest.RequireStatus(t, err, http.StatusConflict)
```

Long form: `$CORE/docs/error-handling.md`, `$CORE/docs/service-errors.md`,
`$CORE/docs/sentry.md`.
