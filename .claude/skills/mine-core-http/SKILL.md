---
name: mine-core-http
description: Write routes, handlers, request binding, pagination and responses in this mine-core service. Use when adding or changing an HTTP endpoint, binding path/query/body params, returning a list with paging, setting a status code, uploading a file, or regenerating the Postman collection.
---

# Routes, handlers and responses

## Routes — one file per module, nothing else in it

```go
// modules/note/note.module.go
func (*Module) Routes(e *core.Server) {
    c := &handler.NoteHandler{}

    e.GET("/notes", c.Pagination, middlewares.AuthRequire(e))
    e.GET("/notes/:id", c.Find, middlewares.AuthRequire(e))
    e.POST("/notes", c.Create, middlewares.AuthRequire(e))
    e.PUT("/notes/:id", c.Update, middlewares.AuthRequire(e))
    e.DELETE("/notes/:id", c.Delete, middlewares.AuthRequire(e))
}
```

**Full paths, no groups, no prefixes, one middleware per line.** A path from a
log or a bug report is greppable and lands on the line that serves it, and
whether a route is protected is answerable without scrolling.

The cost: a route added without `middlewares.AuthRequire(e)` is open. That is
what each module's `Test<Name>API_requiresAuthentication` is for — **add every
new route to it** ([note.module_test.go](../../../modules/note/note.module_test.go)).

`core.NewHTTPServer` already applies request-id, structured request logging,
panic recovery, CORS and the `IError` renderer. A module registers routes and
states its dependencies; it configures no middleware of its own.

## Handlers — bind, convert, call, render

```go
func (m NoteHandler) Create(c core.IHTTPContext) error {
    ownerID, err := callerID(c)          // c.GetUser().ID, guarded
    if err != nil {
        return err
    }

    input := &CreateRequest{}
    if bErr := c.BindWithValidate(input); bErr != nil {
        return bErr                       // 400 with {code, message, fields}
    }

    payload, _ := utils.Copy[service.CreatePayload](input)

    note, sErr := service.NewNoteService(c).Create(ownerID, &payload)
    if sErr != nil {
        return sErr                       // returned as-is; the server renders it
    }

    return c.JSON(http.StatusCreated, note)
}
```

Four things and no more. A line that *decides* anything belongs in the service,
where a job can reach it too. Handlers never log (the request middleware already
logs method/path/status/latency/request-id) and never build an error message —
they return `core.IError` from below unchanged.

Statuses come from `net/http`, never a literal. `201` on create, `204`
+ `c.NoContent` on delete, `200` otherwise.

## Binding

`c.BindWithValidate(input)` binds **and** runs `Valid` in one pass. Use
`c.BindOnly(input)` only when there is genuinely nothing to validate.

Struct tags select the source; a request can mix them, which is how a path id is
validated together with the body:

```go
type UpdateRequest struct {
    ID     *string `param:"id" json:"-"`   // path
    Title  *string `json:"title"`          // body
    Pinned *bool   `json:"pinned"`
}
```

`param:` path · `query:` query string · `json:` body · `form:` form/multipart ·
`header:` header. **Pointer fields**: a `*string` distinguishes "absent" from
"empty", which is what makes `Required()` and partial updates work.

The error renderer reports which source a bad field came from —
`fields["id"].In == "path"`.

Raw accessors when a struct is overkill: `c.Param("id")`, `c.QueryParam("q")`,
`c.FormFile("file")`, `c.Request()`.

## Pagination

```go
opts := c.GetPageOptionsWithAllowed("created_at", "title", "pinned")
page, err := store.Note(c).Scopes(...).Pagination(opts)   // *core.Page[models.Note]
```

`GetPageOptionsWithAllowed` is what makes `order_by` safe — a column outside the
allowlist is dropped instead of reaching SQL. Always prefer it to
`GetPageOptions()`.

Query string: `?page=1&limit=20&order_by=created_at desc&q=text`. The response is
`{items, total, count, page, limit, q, order_by}` — `total` is the whole match,
`count` is this page.

Apply a default order with `repo.DefaultOrder(opts, "pinned DESC", "created_at DESC")` —
**not** `.Order()`, which would emit the clause twice (`Pagination` already
applies `opts.OrderBy`).

## Responses and DTOs

Return what `service/` returns. If the wire shape must differ from the model
(hiding a password hash, flattening a relation), that is a DTO in
`service/<name>.dto.go` — not a map built in the handler.

## Postman

```bash
make postman        # data/postman_collection.generated.json (gitignored)
make postman-flat   # one folder per top-level path segment
```

Generated from the registered routes, so it cannot drift. What this project
names for itself — collection name, base URL, which route captures the token —
is in [postmangen.json](../../../postmangen.json). Run it after adding a route.

## Reference

`sed -n '/^type IHTTPContext interface/,/^}/p' "$(go list -m -f '{{.Dir}}' github.com/pskclub/mine-core/v2)/http_context.go"`
lists every method. Long form: `$CORE/docs/http.md`.

Related skills: `mine-core-validation`, `mine-core-auth`, `mine-core-errors`.
