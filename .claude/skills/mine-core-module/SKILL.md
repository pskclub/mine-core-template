---
name: mine-core-module
description: Build or extend a feature module in this mine-core service — new module scaffold, new endpoint, new layer, wiring into cmd.Modules, and making one module use another. Use whenever adding a feature, a resource, or a use case, or when an arch import-rule test fails.
---

# Adding a feature

Every feature is a **module** under [modules/](../../../modules/). The worked
example is [modules/note](../../../modules/note/) — read
[its README](../../../modules/note/README.md) before the first one you write.

## Start here

```bash
make new-module name=order      # writes the whole shape under modules/order
```

It writes routes, handler, request, service, dto, store and both test files, and
prints the four things it cannot decide. Do those four:

1. the prisma model in `prisma/schema/*.prisma`, then `make migrate-dev`
2. the struct in [models/](../../../models/) **and** its line in `allModels()` in [testkit/testkit.go](../../../testkit/testkit.go)
3. the module's error codes in `emsgs/<name>.emsgs.go`
4. one line in `Modules` in [cmd/modules.go](../../../cmd/modules.go) — `order.New()`

Adding an endpoint to an existing module touches only that module — plus its
`requiresAuthentication` test.

## The layout

```
modules/order/
  order.module.go        core.IModule — everything this module attaches
  order.module_test.go   the whole service over a real listener
  order.api.go           what OTHER modules may call (only if something calls in)
  handler/
    order.handler.go     bind → convert → call → render
    order.request.go     what arrives over HTTP + its Valid rules
  service/
    order.service.go     the business rules; takes core.IContext
    order.dto.go         payloads the service takes — no HTTP in them
    order.deps.go        interfaces this module needs from other modules
    order.job.go         scheduled work, if any
    order.service_test.go
  store/
    order.store.go       the only package that queries `orders`
    order.store_test.go
```

Each layer reaches only the one below: **handler → service → store**. A module
has only the layers it has work for — [modules/home](../../../modules/home/home.module.go)
is one file; [modules/exchange](../../../modules/exchange/) has no `store/`
because its data is an HTTP call.

**File names mirror their directory**: `order.handler.go` in `handler/`,
`order.store.go` in `store/`. A file holding a different kind of thing keeps its
own name (`order.request.go`, `order.dto.go`).

## The import rules — a test, not a convention

[arch/arch_test.go](../../../arch/arch_test.go) runs under `make test` and fails
the build on:

1. `models/` and `consts/` import nothing from this repository.
2. `repo/`, `emsgs/`, `helpers/`, `middlewares/` import no module and no `cmd`.
3. **No module imports another module.**
4. No module imports `cmd`.
5. A module is entered through its **top-level package only** — nothing outside `modules/order` may import `modules/order/service`.
6. `testkit` is imported only from `_test` files.

A module's *external* test package (`package order_test`) may import siblings;
only non-test imports are checked.

## When your module needs another module

Do not import it. Declare **the shape you need** as your own interface in
`service/<name>.deps.go`, and let `cmd.Modules` supply it — this is how auth
reads accounts without importing user:

```go
// modules/order/service/order.deps.go
type Users interface {
    Find(id string) (*models.User, core.IError)
}

// A factory, not a value: every service is bound to one context.
type UsersFor func(core.IContext) Users
```

```go
// cmd/modules.go — the only place that knows both modules
usersFor := func(ctx core.IContext) order.Users { return user.NewUserService(ctx) }

return core.NewModules(
    home.New(),
    order.New(usersFor),
)
```

See [auth.deps.go](../../../modules/auth/service/auth.deps.go). Keep the
interface **narrow** — every method is another thing your module can do to a
table it does not own.

## When another module needs yours

Add `<name>.api.go`: type aliases plus one-line forwards, nothing else. See
[user.api.go](../../../modules/user/user.api.go).

```go
type IOrderService = service.IOrderService

func NewOrderService(ctx core.IContext) IOrderService {
    return service.NewOrderService(ctx)
}
```

Whatever is not listed there, no other module can call. Adding a line is a
deliberate widening of the promise and reads as one in review.

## Wiring

A module declares everything it attaches in its own `<name>.module.go`. Only
`Name` is required; the rest are optional interfaces added as the module grows:

| interface | method | attached when the role has |
|---|---|---|
| `IHTTPModule` | `Routes(e *core.Server)` | an HTTP server |
| `IJobModule` | `Jobs(reg *core.JobRegistry) core.IError` | a job runner |
| `ICronModule` | `Cron(sc *core.Scheduler) core.IError` | a scheduler |
| `IMQModule` | `Consumers(c core.IMQConsumer)` | an MQ consumer |
| `IHealthModule` | `HealthChecks() []core.HealthCheck` | always |
| `ILifecycleModule` | `Start(app)` / `Stop(ctx)` | always |

`Modules` in [cmd/modules.go](../../../cmd/modules.go) is the composition root:
the one function that knows every module. It is exported because `testkit.Serve`
assembles the same set — a module wired wrongly fails a test rather than
production.

```go
func Modules(app *core.App) (*core.ModuleSet, core.IError) {
    usersFor := func(ctx core.IContext) auth.Users { return user.NewUserService(ctx) }
    middlewares.RegisterAuth(app, auth.ResolveToken(app, usersFor))  // before any route

    return core.NewModules(
        home.New(),
        auth.New(usersFor),
        order.New(),
    )
}
```

There is no global registry and no `init()` registration on purpose: this list is
what lets a test assemble the real service minus one module, and an `init()`
takes no arguments, so a module registered by one could never be given
`usersFor`.

**Jobs no longer have a second root.** A module arms its own schedules through
`Cron`, in the same file as its routes — see
[auth.module.go](../../../modules/auth/auth.module.go). `newScheduler` in
[cmd/worker.go](../../../cmd/worker.go) is now only for work that belongs to the
process rather than to any feature.

## Rules that decide where code goes

- **Payload rules go in the request's `Valid`** (answerable from the request alone);
  rules that need rows go in the **service** and return an `emsgs` error.
- **Ownership is an explicit first argument** (`ownerID string`) applied as a
  scope/WHERE clause, re-applied on every statement. "Not yours" answers **404**,
  never 403.
- A **service takes `core.IContext`**, never a concrete type, so the same code
  runs under HTTP, a job or a test with no test-only interfaces.
- A handler does four things: bind, convert (`utils.Copy[service.CreatePayload]`),
  call, render. Any line that *decides* something belongs in the service.
- A **store is the only package that queries its module's tables**, and it is silent —
  no logging (the GORM logger already covers it).

## Finishing

```bash
make test && make lint && gofmt -w .
make postman            # after adding or changing a route
```

Related skills: `mine-core-http`, `mine-core-database`, `mine-core-testing`,
`mine-core-errors`.
