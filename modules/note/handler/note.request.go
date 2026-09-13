package handler

import (
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/valid"
)

const (
	maxTitleLength = 120
	maxBodyLength  = 10000
)

type CreateRequest struct {
	Title  *string `json:"title"`
	Body   *string `json:"body"`
	Pinned *bool   `json:"pinned"`
}

// Valid states what has to be true of the payload itself, and nothing else.
//
// It cannot ask "does this user already have too many notes" — that depends on
// rows, changes between the check and the insert, and belongs to the service.
// The split is worth holding: everything here is answerable from the request
// alone, so it stays correct no matter who calls the service later.
func (r *CreateRequest) Valid(ctx core.IContext) core.IError {
	v := valid.New(ctx)

	v.Str("title", r.Title).Required().Length(1, maxTitleLength)
	v.Str("body", r.Body).Required().Length(1, maxBodyLength)

	// Not Required: a missing `pinned` means false, which is a sensible note.
	// Only fields with no usable default are required.

	return v.Error()
}

type UpdateRequest struct {
	// Bound from the path, then validated with the body in the same pass.
	ID     *string `param:"id" json:"-"`
	Title  *string `json:"title"`
	Body   *string `json:"body"`
	Pinned *bool   `json:"pinned"`
}

func (r *UpdateRequest) Valid(ctx core.IContext) core.IError {
	v := valid.New(ctx)

	v.Str("id", r.ID).Required().UUID()
	v.Str("title", r.Title).Required().Length(1, maxTitleLength)
	v.Str("body", r.Body).Required().Length(1, maxBodyLength)

	return v.Error()
}
