package emsgs

import "github.com/pskclub/mine-core/v2/valid"

// Validation codes raised by the user module through v.Must, which the built-in
// rules do not cover.
const (
	// CodeDuplicateInRequest marks a value repeated within one payload — a clash
	// the database cannot see, because it knows nothing about the other entries
	// being submitted alongside.
	CodeDuplicateInRequest = "DUPLICATE_IN_REQUEST"
)

func init() {
	valid.SetMessage(CodeDuplicateInRequest, "The {field} field is repeated in this request")
}
