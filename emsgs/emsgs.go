// Package emsgs holds this service's own errors and validation messages, one
// file per module: auth.emsgs.go, user.emsgs.go, and so on.
//
// Declaring them here rather than at each call site means a code is written once
// and reused. A code is what clients branch on, so a typo in a repeated string
// literal is a silent break that nothing catches — a shared value cannot drift.
//
// The framework's own errors live in mine-core's errmsgs (NotFound, BadRequest,
// Unauthorized, DBError …). Use those directly; this package is only for what
// this service defines.
//
//	import "<module>/emsgs" // <module> is the path in go.mod
//
//	return emsgs.InvalidCredentials
//
// # Conventions
//
// Statuses come from net/http, never a literal: http.StatusUnauthorized says
// what 401 means without the reader having to recall the number.
//
// Errors are sentinels — compare with errors.Is, which matches by code through
// any wrapping. Every builder (WithMessage, WithFields …) returns a copy, so a
// caller can enrich one without disturbing the shared value.
//
// A validation code raised with v.Must has no message of its own, so each file
// registers its own in an init. Keeping the code and its wording together means
// using the code requires importing this package, which runs that init — a code
// cannot be raised without its message existing.
package emsgs
