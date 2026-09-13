package handler

import (
	"github.com/pskclub/mine-core-template/models"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/valid"
)

// minPasswordLength is the floor, not the policy. Length is the one rule that
// reliably helps; composition rules mostly push people toward "P@ssw0rd1".
const minPasswordLength = 8

type RegisterRequest struct {
	Email    *string `json:"email"`
	FullName *string `json:"full_name"`
	Password *string `json:"password"`
}

func (r *RegisterRequest) Valid(ctx core.IContext) core.IError {
	v := valid.New(ctx)

	v.Str("email", r.Email).Required().Email().
		Unique(models.User{}.TableName(), "email", valid.Cond("deleted_at IS NULL"))

	v.Str("full_name", r.FullName).Required().Length(2, 100)
	v.Str("password", r.Password).Required().Length(minPasswordLength, 72)

	return v.Error()
}

type LoginRequest struct {
	Email    *string `json:"email"`
	Password *string `json:"password"`
}

// Valid checks only that the fields are present and shaped like credentials.
// Whether they are *correct* is the service's job, and it answers with one
// message for every failure so this endpoint cannot be used to discover which
// addresses are registered.
func (r *LoginRequest) Valid(ctx core.IContext) core.IError {
	v := valid.New(ctx)

	v.Str("email", r.Email).Required().Email()
	v.Str("password", r.Password).Required()

	return v.Error()
}
