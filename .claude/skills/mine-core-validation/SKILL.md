---
name: mine-core-validation
description: Write request validation with mine-core's valid package — Valid/Validate methods, field rules, uniqueness and existence checks against the database, conditional and cross-field rules, nested and array payloads, custom codes and messages. Use when adding or changing a request struct, or when deciding whether a rule belongs in the request or the service.
---

# Validation

## Where a rule goes

**In the request's `Valid`** if it is answerable from the payload alone —
"a title is 1–120 characters". **In the service** if it needs rows —
"you already have 200 notes" — because that changes between the check and the
insert. Service rules return an `emsgs` sentinel, usually 409, not a field
error: nothing about what was sent is wrong.

Uniqueness is the exception that lives in the request: `Unique`/`Exists` are
database rules the validator runs for you, and a 400 naming the field is a
better answer than a 500 from the unique index.

## The shape

```go
type CreateRequest struct {
    Title  *string `json:"title"`
    Body   *string `json:"body"`
    Pinned *bool   `json:"pinned"`
}

func (r *CreateRequest) Valid(ctx core.IContext) core.IError {
    v := valid.New(ctx)

    v.Str("title", r.Title).Required().Length(1, maxTitleLength)
    v.Str("body", r.Body).Required().Length(1, maxBodyLength)
    // Not Required: a missing `pinned` means false, which is a sensible note.

    return v.Error()
}
```

`c.BindWithValidate(input)` binds and calls `Valid` in one pass, and the server
renders violations as `{code, message, fields}` with the source of each field
(`fields["id"].In == "path"`).

**Pointer fields are what make this work** — `*string` distinguishes absent from
empty. Only fields with no usable default are `Required()`.

## The rules

```go
v.Str(name, *string)   Required Length(min,max) Min Max Email URL UUID IP JSON Base64
                       Numeric Lowercase Uppercase In(...) Contains NotContains
                       Prefix Suffix Match(*regexp.Regexp) Date(layout...) DateTime ISO8601
                       Unique(table, col, scopes...) Exists(table, col, scopes...)
                       MongoUnique(coll, filter) MongoExists(coll, filter)
                       Custom(code, ok) Check(code, func(ctx, string) (bool, error))
                       Message(msg)
v.Int(name, *int64)    Required Min Max Between Positive In(...) Message
v.Float(name, *float64) — same as Int
v.Bool(name, *bool)    Required Message
v.Time(name, *time.Time) Required Before After Between Message
v.Arr(name, slice)     Required Min Max Size Message
```

Composition:

```go
v.When(cond, func(v *valid.Validator) { ... })        // conditional group
v.Must(field, code, ok, data...)                      // any predicate you can write
v.CheckDB(field, code, func(ctx) (bool, error))       // any query you can write
v.Nested("address", r.Address)                        // one sub-struct
valid.EachNested(v, "users", r.Users)                 // a slice of them
v.Each(name, len(items), func(iv *valid.Validator, i int) { ... })
```

Database scopes for `Unique`/`Exists`:

```go
v.Str("email", r.Email).Required().Email().
    Unique(models.User{}.TableName(), "email", valid.Cond("deleted_at IS NULL"))

// on update, ignore the row being edited
    Unique("users", "email", valid.Except("id", *r.ID))
```

## Reusable sub-structs

A type that describes the same thing in several requests puts its rules in
`Validate(v *valid.Validator)` instead of `Valid`, so it can be nested:

```go
func (r *CreateBulkItem) Validate(v *valid.Validator) {
    v.Str("email", r.Email).Required().Email().Unique("users", "email")
    v.Str("full_name", r.FullName).Required().Length(2, 100)
}

func (r *CreateBulkRequest) Valid(ctx core.IContext) core.IError {
    v := valid.New(ctx)
    v.Arr("users", r.Users).Required().Min(1).Max(maxBulkUsers)
    valid.EachNested(v, "users", r.Users)   // violations land under users.2.email
    return v.Error()
}
```

Worked example, including the duplicate-within-one-payload check that no
database rule can make:
[user_create_bulk.request.go](../../../modules/user/handler/user_create_bulk.request.go).

## Codes and messages

Built-in codes (`REQUIRED`, `INVALID_UUID`, `INVALID_STRING_LENGTH`, `UNIQUE`,
`NOT_EXISTS`, …) carry their own message. A code you raise yourself with
`v.Must` does not, so **register it in `emsgs/<module>.emsgs.go`**, in an
`init` next to the constant:

```go
const CodeDuplicateInRequest = "DUPLICATE_IN_REQUEST"

func init() {
    valid.SetMessage(CodeDuplicateInRequest, "The {field} field is repeated in this request")
}
```

Keeping the code and its wording together means raising the code requires
importing the package, which runs the init — a code cannot exist without its
message. `{field}` plus any keys from the rule's `data` map are interpolated.

## Testing

Assert **codes**, not messages — messages are wording, codes are the contract:

```go
codes := c.Post("/notes", `{}`).RequireStatus(http.StatusBadRequest).FieldCodes()
assert.Equal(t, "REQUIRED", codes["title"])

fields := c.Put("/notes/not-a-uuid", body).RequireStatus(400).Error().Fields
assert.Equal(t, "INVALID_UUID", fields["id"].Code)
assert.Equal(t, "path", fields["id"].In)
```

Full rule list: `$CORE/docs/validation.md`, or
`grep -h "^func " "$CORE/valid"/*.go`.
