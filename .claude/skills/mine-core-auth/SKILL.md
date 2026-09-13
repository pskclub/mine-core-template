---
name: mine-core-auth
description: Protect routes and work with the signed-in caller in this mine-core service — AuthRequire/AuthOptional/AuthRole, RegisterAuth wiring, the opaque bearer token scheme, roles, and ownership checks. Use when adding a protected route, changing who may call something, touching the auth module, or debugging a 401/403.
---

# Authentication and authorization

## Protecting a route

```go
e.GET("/notes/:id", c.Find, middlewares.AuthRequire(e))          // 401 without a valid token
e.GET("/home", c.Index, middlewares.AuthOptional(e))             // c.GetUser() may be nil
e.DELETE("/users/:id", c.Delete, middlewares.AuthRole(e, "admin"))
```

(`consts/` currently defines only the process roles — `RoleAPI`, `RoleWorker`,
`RoleAll`. Caller roles are not declared yet; add them there, not per module, the
first time two packages have to agree on one.)

Written on **each route**, one middleware per line — never on a group. A group
would protect a later route by default, but then the only way to know whether a
route is guarded is to scroll up. The safety net is each module's
`Test<Name>API_requiresAuthentication`, which names every route and asserts an
anonymous caller gets 401. **Every new route goes into that test.**

`AuthRole` answers only "who is calling, in what role". Whether that caller may
touch *this row* is a business rule and belongs in the service, where it can see
the row.

## Reading the caller

```go
user := c.GetUser()          // *core.ContextUser: ID, Email, Username, Name, Segment, Token, Data
```

Guard it even behind `AuthRequire` — a route registered without the middleware
should answer 401 rather than panic, and that is the one thing a handler logs:

```go
func callerID(c core.IHTTPContext) (string, core.IError) {
    user := c.GetUser()
    if user == nil || user.ID == "" {
        c.Log().Error("route reached with no caller",
            "module", "note", "path", c.Request().URL.Path,
            "hint", "the route is missing middlewares.AuthRequire(e)")
        return "", errmsgs.Unauthorized
    }
    return user.ID, nil
}
```

Then **pass the id down as an explicit first argument** —
`Find(ownerID, id string)`. A service that reads `c.GetUser()` itself cannot be
called from a job or an admin tool, and a caller can forget an ownership check
that is not in the signature. See `mine-core-database` for how it is applied.

## How the wiring works

`middlewares` cannot import `auth` — every module's routes import `middlewares`,
and auth's own routes sit behind it, so the import would close a cycle. Instead
`cmd.Modules` hands it the finished token lookup at startup:

```go
usersFor := func(ctx core.IContext) auth.Users { return user.NewUserService(ctx) }
middlewares.RegisterAuth(app, auth.ResolveToken(app, usersFor))   // before any route
```

The guard is stored **per `*core.App`**, not in a global — every test builds its
own App against its own database, and a single global would authenticate one
parallel test's requests against another's data. A server built without
`RegisterAuth` gets a middleware that refuses every request and logs
`authentication is not configured` at registration time.

[middlewares/auth.go](../../../middlewares/auth.go) ·
[auth.api.go](../../../modules/auth/auth.api.go)

## The token scheme

Opaque bearer tokens verified against the database — **not JWT**.

- 256 bits from `crypto/rand`; only the **SHA-256 digest** is stored, and
  `core.HashedTokenVerifier` hashes what the request carried before the lookup,
  so a plaintext token never reaches the database and a leaked dump yields
  nothing that resolves.
- Plain SHA-256 (not bcrypt) is deliberate **for tokens** — high entropy, looked
  up on every request. **Passwords use bcrypt** (`utils.HashPassword` /
  `utils.ComparePassword`).
- TTL is `TokenTTL` in [auth.service.go](../../../modules/auth/service/auth.service.go)
  (24h, deliberately not re-exported). Revocation is a `DELETE`.
- An expired row still present answers `TOKEN_EXPIRED` ("sign in again");
  once purged the same token answers `UNAUTHORIZED`. That is why
  `auth.purge-expired-tokens` keeps rows for a week after expiry.
- **Sign-in answers identically for a wrong password and an unknown address.**
  Keep it that way — the difference is an account-enumeration oracle.

Never log a token, its digest, or a password hash.

## Testing behind the guard

```go
anon, app := testkit.Serve(t)      // whole service, real listener
c := testkit.SignIn(t, anon)       // registers + logs in, returns a client with the token
```

`SignIn` goes through `/auth/register` and `/auth/login` rather than inserting a
row and forging a token: those are the only supported ways to get one, and a
forged token would keep passing after the real path broke. A second account:
see `signInAs` in [note.module_test.go](../../../modules/note/note.module_test.go).

Long form: `$CORE/docs/auth.md`, `$CORE/docs/middleware.md`.
