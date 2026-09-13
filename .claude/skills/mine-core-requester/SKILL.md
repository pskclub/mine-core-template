---
name: mine-core-requester
description: Call another service or a third-party API from this mine-core service — core.Requester, typed success and failure bodies, per-call timeouts, mapping an upstream's outcome to our own errors, and testing against an httptest server. Use whenever code needs to make an outbound HTTP request.
---

# Calling other services

**Always `core.Requester(ctx)`.** Never `http.Get`, never a resty client built
for the occasion. What the shared one carries:

- **the caller's deadline** — a request that has been abandoned stops the call it
  was waiting on, instead of holding a connection for the upstream's timeout
- **the caller's trace** — every call leaves a breadcrumb on the request that
  made it, so a Sentry issue shows what we asked and what came back without
  anyone logging it by hand
- **one client, one connection pool**, configured once at startup

None of that survives a client built per call.

## The shape

```go
// A context of our own, so this one call is bounded without touching the
// shared client's timeout. Requester takes any context — that is why it is a
// function and not a method on IContext.
ctx, cancel := context.WithTimeout(s.ctx, upstreamTimeout)
defer cancel()

r := core.Requester(ctx)

var out ratesResponse
var fail upstreamError

resp, err := r.Send(
    r.R().
        SetQueryParams(map[string]string{"base": base, "symbols": quote}).
        SetResult(&out).      // body on 2xx
        SetError(&fail),      // body on non-2xx
    http.MethodGet, baseURL+"/latest",
)
if err != nil {
    return nil, s.upstreamFailed(err, &fail, base, quote)
}
```

`r.R()` is the full go-resty request API (`SetBody`, `SetHeader`,
`SetQueryParams`, `SetPathParams`, `SetFormData`, `SetFileReader`…). `Send`
returns a `core.IError` on a transport failure (`NETWORK_ERROR`, 500) or on any
non-2xx, reusing the remote code and message when the body carried them.

**Use `SetResult` and `SetError` so both bodies decode into our own types** —
the failure body is where a provider puts anything a code and a message cannot.

Per-call timeouts belong here; client-wide settings (retries, TLS, base URL,
middleware) go on `core.Requester(ctx).Resty()` **once at startup**, never per
request.

## Two checks the status code does not make

```go
if out.Rates == nil {
    // 200 with no `rates` key at all — so this is not a rates response.
    // A provider that has started demanding an API key answers exactly like this.
    s.ctx.Log().Error("provider answered 200 without a rates object", ...)
    return nil, emsgs.ExchangeUnavailable
}

rate, ok := out.Rates[quote]
if !ok {
    // Present but not this pair: that IS an answer and the caller can act on it.
    s.ctx.Log().Warn("quote missing from a successful response", ...)
    return nil, emsgs.UnknownCurrency.WithFields(map[string]any{"quote": quote})
}
```

The distinction is the point: a body missing the key is **theirs** to fix, a body
with the key but not the value is the **caller's**. Collapsing the two is how a
missing credential gets reported to a client as "that currency pair is not
available". And reading a value out of a map without checking `ok` is how a
missing quote becomes a rate of `0.00` three layers away.

Note also: `SetError` binds on non-2xx only, so on a 200 the failure struct is
still the zero value — logging its fields there prints two empty strings.

## Map every outcome to an error we own

An upstream code must never reach our clients: a caller branching on the
provider's codes breaks the day we change provider. Keep the original as the
cause, for the log and for Sentry.

```go
func (s exchangeService) upstreamFailed(err core.IError, fail *upstreamError, base, quote string) core.IError {
    log := s.ctx.Log().With("base", base, "quote", quote,
        "upstream_status", err.GetStatus(), "upstream_code", err.GetCode())

    switch {
    case fail.IsUnknownCurrency() || err.GetStatus() == http.StatusNotFound:
        log.Warn("unknown currency pair")
        return emsgs.UnknownCurrency.WithFields(...)          // their 404 → our 400

    case err.GetStatus() == http.StatusTooManyRequests || fail.Retryable:
        log.Warn("rate provider is throttling us")
        return s.ctx.NewError(err, emsgs.ExchangeUnavailable) // our quota → our 503

    default:
        return s.ctx.NewError(err, emsgs.ExchangeUnavailable) // their 5xx, timeout, refused
    }
}
```

Full worked example with the reasoning inline:
[exchange.service.go](../../../modules/exchange/service/exchange.service.go).

## Configuration and caching

The base URL is a module key, read ad hoc — `ctx.ENV().String("exchange_base_url")` —
and **missing config fails loudly** rather than calling a default nobody chose.

Cache what is expensive and slow-moving, and keep working without redis:

```go
cache := s.ctx.Cache()
if cache == nil {
    return s.fetch(base, quote)
}
return core.Remember(cache, cacheKey(base, quote), rateTTL, func() (*Rate, error) {
    return s.fetch(base, quote)
})
```

`core.Remember` preserves a load failure's status and code, so the mapping above
survives the cache layer intact.

## Logging

The requester logs outgoing calls itself — level via `HTTP_LOG_LEVEL`
(`warn` by default: slow calls and failures), bodies via `HTTP_LOG_BODY` (off,
because a body carries credentials). Do not log the call; log the **decision**
you made about its outcome.

## Testing

Point config at an `httptest.Server` — **never mock `IRequester`**. Both ends stay
real, so the test covers the query parameters sent, the JSON decoded and the
statuses mapped:

```go
srv := httptest.NewServer(http.HandlerFunc(h))
t.Cleanup(srv.Close)

ctx := testkit.Context(t, coretest.WithEnv(map[string]string{"exchange_base_url": srv.URL}))
```

Cover: the success path, the 4xx you translate, the 5xx/timeout, a malformed
200, and that a second call inside the TTL does not hit the server.
[exchange.service_test.go](../../../modules/exchange/service/exchange.service_test.go).

Long form: `$CORE/docs/requester.md` (400 lines).
