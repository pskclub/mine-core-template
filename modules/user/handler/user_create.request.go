package handler

// Request payloads and their validation rules live beside the handler that
// binds them. A payload binds from the path, query and body (see the struct
// tags) and states its rules in Valid; the framework renders any violation as
// {code, message, fields:{field:{code, message, in}}}.
//
// Error codes and their messages live in emsgs, not here — they are the
// contract with the client, and a code written twice is a code that can drift.

import (
	"github.com/pskclub/mine-core-template/models"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/valid"
)

type CreateRequest struct {
	Email    *string `json:"email"`
	FullName *string `json:"full_name"`
}

// Valid runs on BindWithValidate. Rules stop at the first failure per field, and
// nil/empty values skip every rule except Required — so optional fields only
// validate when they are actually sent.
func (r *CreateRequest) Valid(ctx core.IContext) core.IError {
	v := valid.New(ctx)

	// Unique runs a query; a soft-deleted row must not block the address, hence
	// the scope. A failing query surfaces as a 500 instead of passing silently.
	v.Str("email", r.Email).Required().Email().
		Unique(models.User{}.TableName(), "email", valid.Cond("deleted_at IS NULL"))

	v.Str("full_name", r.FullName).Required().Length(2, 100)

	return v.Error()
}
