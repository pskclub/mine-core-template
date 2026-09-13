---
name: mine-core-infra
description: Use mine-core's optional backends from this service — Redis cache (Remember, counters, locks, pub/sub), S3 storage and presigned uploads, RabbitMQ publish/consume, mailer and FCM push, plus health probes and graceful shutdown. Use when caching, storing a file, sending a message, an email or a notification, or wiring a new backend.
---

# Cache, storage, messaging, mail, push

All of these hang off `IContext` and are **optional**. A backend with no config
is never opened ([cmd/bootstrap.go](../../../cmd/bootstrap.go)), and nothing
returns nil:

- `ctx.Cache()` — misses every read, drops every write. **Cache-aside code runs
  unchanged without redis.** Ask `.Enabled()` only when a path genuinely needs one.
  (mine-core documents this handle as never nil; the exchange service still
  nil-checks it defensively — either is safe, and a nil check is not a substitute
  for `.Enabled()`.)
- `ctx.Storage()`, `ctx.MQ()` — every call **fails** naming the missing
  configuration. A file silently not stored, or a message nobody receives, is not
  something to degrade quietly about.

---

## Cache (Redis)

```go
core.Remember(cache, key, ttl, func() (*Rate, error) { return s.fetch(...) })
core.RememberOnce(cache, key, ttl, wait, load)   // single-flight: one loader, others wait
```

`Remember` preserves a load failure's status and code, so error mapping survives
the cache layer.

```go
cache.Get(key, &dest)                      // *string/*[]byte raw, anything else JSON
cache.Set(key, v, ttl)                     // core.NoExpiry, core.KeepTTL
cache.SetNX(key, v, ttl) (bool, IError)    // idempotency keys, one-shot flags
cache.GetDel(key, &dest)                   // OTPs and single-use tokens — no replay
cache.MGet / MSet / Del / DelByPrefix / Exists / Expire / TTL
cache.Incr(key, delta, ttl)                // creates with the ttl → a rate limiter in one call
cache.Lock(key, ttl) (ILock, IError)       // wraps ErrLockNotAcquired when held
cache.WithPrefix("otp")                    // namespaced handle
```

Miss is an error wrapping `core.ErrCacheMiss` — test with `errors.Is`, do not
treat every failure as a miss.

**Key conventions**: `module:thing:id`, built by a helper in the module that owns
it (`func cacheKey(base, quote string) string`), never inline. TTL belongs next
to the key as a named constant with the reason it has that value.

**Never cache authorization decisions or another module's rows.** Cache what is
expensive and slow-moving, and make the code correct with the cache switched off.

`$CORE/docs/cache*.md` — operations, counters, locks, patterns, testing.

## Pub/Sub (redis fan-out)

```go
ctx.PubSub().Publish("order.created", payload)
sub, _ := ctx.PubSub().Subscribe("order.created")     // or PSubscribe("order.*")
```

Fire-and-forget, no delivery guarantee — for cache invalidation and live
updates, never for work that must happen. `$CORE/docs/pubsub*.md`.

## Message queue (RabbitMQ)

```go
ctx.MQ().Publish(exchange, routingKey, msg)          // waits for the broker's confirm
ctx.MQ().DeclareExchange(cfg); DeclareQueue(cfg); BindQueue(q, ex, key)
```

Declare topology at startup, not per publish — declaring an existing object with
different settings is an error from the broker, which is the point: it catches a
topology change nobody applied. Consumers run under the worker role and get an
`IMQContext` with the same capabilities as a handler. `$CORE/docs/mq.md`.

## Storage (S3)

```go
ctx.Storage().Put(key, reader)          // multipart for large bodies
ctx.Storage().PutBytes(key, data)
rc, err := ctx.Storage().Get(key)       // close it; wraps ErrObjectNotFound
ctx.Storage().Stat / Exists / List(prefix) / Copy / Move / Delete / DeleteByPrefix
ctx.Storage().WithPrefix("tenants/42")
```

**Prefer presigned links to proxying bytes through the service:**

```go
url, err := ctx.Storage().PresignPut(key, 15*time.Minute)   // browser uploads straight to the bucket
url, err := ctx.Storage().PresignGet(key, 5*time.Minute)    // a reader with no credentials
```

The client must send the same `Content-Type` the PUT link was signed with.
`PublicURL(key)` is the unsigned address (a CDN when `S3_PUBLIC_URL` is set) and
says nothing about whether the object is actually readable — that is the
bucket's policy.

**The key is a decision, not an accident**: derive it in the owning module
(`notes/<note_id>/<uuid>.<ext>`), never from the client's filename.
`$CORE/docs/storage*.md`.

## Mail and push

`ctx` does not carry them — they hang off the `App` and are reached where the App
is available. Both have memory backends for tests, so an assertion is on what
*would* have been sent. Templates and payload building:
`$CORE/docs/mailer.md`, `$CORE/docs/push.md`.

Never put a token, a password or a one-time code in a log line on the way out.

---

## Health and shutdown

`core.LiveHandler` / `core.ReadyHandler` answer `/healthz` and `/readyz` over the
App's own dependencies — readiness pings every configured backend, so adding one
extends the probe automatically. e2e waits on `/healthz`
([e2e_test.go](../../../e2e/e2e_test.go)).

`core.Runner` starts the HTTP server, the scheduler and any consumers together
and stops them **in order**: stop accepting, drain requests, finish jobs, then
close the pools. That ordering is why `newScheduler` hands its job runner over
rather than letting the scheduler own it. `$CORE/docs/health.md`,
`runner.md`, `deployment.md`.

## Adding a backend

1. `core.NewX(env)` in [cmd/bootstrap.go](../../../cmd/bootstrap.go), guarded by
   its config being present, appended as `core.WithX(...)`.
2. The keys in [.env.sample](../../../.env.sample), commented.
3. The service in [docker-compose.yml](../../../docker-compose.yml) and the CI
   variables in [.github/workflows/ci.yml](../../../.github/workflows/ci.yml).
4. Tests: use the memory backend, or point at a container behind
   `--tags=integration`.
