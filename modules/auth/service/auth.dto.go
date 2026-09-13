package service

import "github.com/pskclub/mine-core-template/models"

type RegisterPayload struct {
	Email    string
	FullName string
	Password string
}

type LoginPayload struct {
	Email    string
	Password string
}

// AuthResult is what a successful sign-in returns: the token and the user in one
// flat object, so a client reads `id` and `email` at the same level as `token`
// instead of reaching into a nested one.
//
// The user is embedded, which is what flattens it — encoding/json promotes an
// anonymous struct's fields to the enclosing object. Password carries `json:"-"`,
// so the hash cannot leak through here any more than it can through the model
// itself.
//
// User must be non-nil: marshalling a nil embedded pointer panics, since there
// are no promoted fields to read. Login always sets it.
type AuthResult struct {
	*models.User

	Token     string `json:"token"`
	ExpiresIn int64  `json:"expires_in"`
}
