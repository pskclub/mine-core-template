# The example module

`modules/note` is a real, working feature — notes belonging to the signed-in
user — kept small on purpose. Read it before writing your first module here; it
is the shape every other one follows.

To start a new module, run `make new-module name=order`. It writes this same
layout with the names substituted.

## The layout

```
modules/note/
  note.module.go            core.IModule — the module itself: New, Name
  note.http.go              its routes, and nothing else
  note.module_test.go
  handler/
    note.handler.go         bind → convert → call → render
    note.request.go         what arrives over HTTP, and its validation rules
  service/
    note.service.go         the business rules
    note.dto.go             what the service takes — no HTTP in it
    note.service_test.go    rules, tested without HTTP
  store/
    note.store.go           how this module queries its own table
```

Each layer may reach only the one below it: **handler → service → store**. The
module's own directory holds every point where it attaches to the service — one
file per kind — so nothing this module registers lives anywhere else.

A file is named after the directory it sits in — `note.store.go` in `store/`, not
`note.repo.go` — so there is exactly one word for each layer. Files that hold a
*different* kind of thing keep their own name: `note.request.go` and
`note.dto.go` are not handlers and services, so they do not pretend to be. The
same rule splits the top level: `note.http.go` holds routes, so its name is
still true when the module grows a cron.

A module has as many of these as it has work for — `modules/home` is a single
file. Three more appear when they are needed:

- `<name>.api.go` — what *other modules* may call, forwarding to `service/`. Only
  when something else actually calls in; see
  [user.api.go](../user/user.api.go). This module has none.
- `<name>.jobs.go` — the `Jobs` and `Cron` methods, arming scheduled work. See
  [auth.jobs.go](../auth/auth.jobs.go); the entry point it arms is a handler
  ([auth.job.go](../auth/handler/auth.job.go)), for the same reason an HTTP one
  is, and the rule it runs stays in `service/`.
- `<name>.mq.go` — the `Consumers` method, for queues this module reads.

## The six rules

### 1. A module owns its tables, and nothing else touches them

`store/` is the only package that queries `notes`, so the ownership filter in
`OwnedBy` cannot be forgotten by someone writing a query elsewhere.

A module that needs a row from another module's table asks through that module's
service interface, and gets back only what that module chose to offer. This is
the rule that keeps a column changeable a year from now, when there are thirty
modules and no one remembers who reads what.

Structs in `models/` are shared, because a type is not a decision. Behaviour is.

### 2. A module is entered through its top-level package only

Nothing outside `modules/note` imports `modules/note/service`. Splitting a module
into directories is an arrangement, not a contract — and an arrangement you can
change without hunting down callers is the point of having one.

What a module offers goes in `<name>.api.go`, which is a list of aliases and
one-line forwards. Adding to it is a deliberate widening of the promise, and
reads as one in review.

### 3. No module imports another module

`modules/user` does not import `modules/auth`, and `modules/auth` does not import
`modules/user` — even though auth needs to read accounts and user's routes need
auth's guard.

Each declares what it needs as an interface of its own, and
[cmd.Modules](../../cmd/modules.go) supplies it — see
[auth/service/auth.deps.go](../auth/service/auth.deps.go), which is the pattern
to copy when your module needs another one.

Authentication is the same idea from the other end: every module's routes call
[middlewares.AuthRequire](../../middlewares/auth.go), and that package imports no
module at all — httpapi hands it the token lookup at startup.

Importing a sibling directly works exactly once. The second such import, in the
other direction, stops compiling and has to be untangled under deadline.
[arch](../../arch/arch_test.go) fails the build before that
happens.

### 4. Routes are written out in full, one middleware per line

```go
e.GET("/notes/:id", c.Find, middlewares.AuthRequire(e))
```

not a group with a prefix. A path seen in a log or a bug report can be grepped
for and lands on the line that serves it, and whether a route is protected is
answerable without scrolling up.

The cost is that a route added without `middlewares.AuthRequire(e)` is not
protected. That is what `TestNoteAPI_requiresAuthentication` is for — it names every route and
asserts each one is closed to an anonymous caller.

### 5. Ownership is a WHERE clause, and "not yours" answers 404

Every method of `INoteService` takes `ownerID` first. Threading it through the
signature rather than reading `c.GetUser()` inside means a job or an admin tool
can call the same code, and that no caller can forget it.

It is applied as a scope, not as a check after loading. Fetching the row and
comparing `user_id` afterwards would work, but 403 confirms the id names a real
note — which is a fact the caller has no right to. Same answer for "not there"
and "not yours".

### 6. Payload rules go in the request; rules that need rows go in the service

`Valid` can say a title is 1–120 characters. It cannot say "you already have 200
notes" — that depends on rows, and changes between the check and the insert. That
one is in `Create`, and returns `emsgs.NoteLimitReached`, a 409: nothing about
what was sent is wrong.

## Two traps worth knowing

**`Updates` with a struct skips zero values.** Unpinning a note — `pinned` going
`true` → `false` — is silently dropped. Any column that can legitimately hold a
zero value has to be written with a map. See `Update` in
[note.service.go](service/note.service.go).

**`repo.DefaultOrder`, not `.Order()`.** `Pagination` already applies
`opts.OrderBy`; adding an `Order()` to the chain emits the clause twice.

## Adding a table

Only `prisma/schema/*.prisma` describes the database — nothing in Go migrates it.
Write the model file, run the migration, then add the matching struct in
`models/` and register it in
[testkit](../../testkit/testkit.go) so the sqlite suite builds
the table too.
