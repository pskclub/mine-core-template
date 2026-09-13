package emsgs

import (
	"net/http"

	core "github.com/pskclub/mine-core/v2"
)

// Errors raised by the note module.
var (
	// NoteLimitReached is a business rule, not a validation rule, which is why
	// it is raised in the service rather than in Valid: the request is
	// well-formed, and whether it is allowed depends on rows the payload cannot
	// see. 409 rather than 400 says the same thing — nothing about what was
	// sent is wrong.
	NoteLimitReached = core.New(
		http.StatusConflict,
		"NOTE_LIMIT_REACHED",
		"you have reached the maximum number of notes",
	)
)
