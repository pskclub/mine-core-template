package emsgs

import (
	"net/http"

	core "github.com/pskclub/mine-core/v2"
)

// Errors raised by the auth module: registering, signing in, and every request
// that carries a token.
var (
	// InvalidCredentials is the single answer to every failed sign-in.
	//
	// Deliberately one error for "no such account" and "wrong password":
	// distinguishing them lets anyone discover which addresses are registered,
	// and the caller can do nothing useful with the difference.
	InvalidCredentials = core.New(
		http.StatusUnauthorized,
		"INVALID_CREDENTIALS",
		"email or password is incorrect",
	)

	// TokenExpired is for a token that verified but has outlived its lifetime.
	//
	// Distinct from a generic rejection because the caller can act on it: the
	// token was real, it has simply run out, and the answer is to sign in again
	// rather than to go looking for a bug. Safe to disclose — it is the holder's
	// own token.
	TokenExpired = core.New(
		http.StatusUnauthorized,
		"TOKEN_EXPIRED",
		"the session has expired, please sign in again",
	)
)
