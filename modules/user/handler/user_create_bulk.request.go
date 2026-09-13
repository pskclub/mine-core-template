package handler

import (
	"strconv"
	"strings"

	"github.com/pskclub/mine-core-template/emsgs"
	"github.com/pskclub/mine-core-template/models"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/valid"
)

// maxBulkUsers bounds one request. Without a ceiling a single call could ask for
// a million inserts in one transaction and hold locks for the duration.
const maxBulkUsers = 100

type CreateBulkRequest struct {
	Users []*CreateBulkItem `json:"users"`
}

// CreateBulkItem is one entry of the batch. Its rules live in Validate so the
// same type can be reused wherever a user is described.
type CreateBulkItem struct {
	Email    *string `json:"email"`
	FullName *string `json:"full_name"`
}

func (r *CreateBulkItem) Validate(v *valid.Validator) {
	v.Str("email", r.Email).Required().Email().
		Unique(models.User{}.TableName(), "email", valid.Cond("deleted_at IS NULL"))

	v.Str("full_name", r.FullName).Required().Length(2, 100)
}

func (r *CreateBulkRequest) Valid(ctx core.IContext) core.IError {
	v := valid.New(ctx)

	v.Arr("users", r.Users).Required().Min(1).Max(maxBulkUsers)

	// EachNested prefixes every violation with its index, so the caller learns
	// which entry to fix: users.2.email rather than a bare "email".
	valid.EachNested(v, "users", r.Users)

	r.checkDuplicateEmails(v)

	return v.Error()
}

// checkDuplicateEmails catches addresses repeated *within* the request.
//
// Unique only asks the database, which knows nothing about the other entries in
// this payload — so a batch containing the same address twice passes every
// per-item rule and then fails on the unique index at insert time, as a 500.
// Catching it here keeps it a 400 that names the offending entry.
func (r *CreateBulkRequest) checkDuplicateEmails(v *valid.Validator) {
	seen := make(map[string]int, len(r.Users))

	for i, u := range r.Users {
		if u == nil || u.Email == nil {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(*u.Email))
		if key == "" {
			continue
		}

		if first, dup := seen[key]; dup {
			v.Must(indexedField("users", i, "email"), emsgs.CodeDuplicateInRequest, false,
				map[string]any{"first": first})
			continue
		}
		seen[key] = i
	}
}

// indexedField builds the path the validator uses for an element of a slice,
// matching EachNested's "name.<index>.field" so both kinds of violation on the
// same entry land under one key.
func indexedField(name string, index int, field string) string {
	return name + "." + strconv.Itoa(index) + "." + field
}
