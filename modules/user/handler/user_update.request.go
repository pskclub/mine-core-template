package handler

import (
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/valid"
)

type UpdateRequest struct {
	// Bound from the path: BindWithValidate reads path params, query and body in
	// one pass, so the route id is validated like any other field.
	ID       *string `param:"id" json:"-"`
	FullName *string `json:"full_name"`
}

func (r *UpdateRequest) Valid(ctx core.IContext) core.IError {
	v := valid.New(ctx)

	v.Str("id", r.ID).Required().UUID()
	v.Str("full_name", r.FullName).Required().Length(2, 100)

	return v.Error()
}
