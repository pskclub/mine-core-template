package handler_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pskclub/mine-core-template/models"
	"github.com/pskclub/mine-core-template/modules/user/handler"
	"github.com/pskclub/mine-core-template/testkit"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/utils"
)

// fieldsOf renders the error the way a client receives it and returns the
// per-field map, so these tests assert on the response rather than internals.
func fieldsOf(t *testing.T, err core.IError) map[string]any {
	t.Helper()
	require.NotNil(t, err)

	raw, mErr := json.Marshal(err.JSON())
	require.NoError(t, mErr)

	var body struct {
		Fields map[string]any `json:"fields"`
	}
	require.NoError(t, json.Unmarshal(raw, &body))
	require.NotNil(t, body.Fields, "expected a fields map")

	return body.Fields
}

func item(email, name string) *handler.CreateBulkItem {
	return &handler.CreateBulkItem{Email: utils.ToPointer(email), FullName: utils.ToPointer(name)}
}

func TestCreateBulkRequest_valid(t *testing.T) {
	ctx := testkit.Context(t)

	r := &handler.CreateBulkRequest{Users: []*handler.CreateBulkItem{
		item("one@example.com", "One"),
		item("two@example.com", "Two"),
	}}

	assert.Nil(t, r.Valid(ctx))
}

func TestCreateBulkRequest_requiresAtLeastOne(t *testing.T) {
	ctx := testkit.Context(t)

	err := (&handler.CreateBulkRequest{}).Valid(ctx)

	require.NotNil(t, err)
	assert.Contains(t, fieldsOf(t, err), "users")
}

// Violations name the entry they belong to, so a caller with fifty users knows
// which one to fix.
func TestCreateBulkRequest_reportsTheOffendingIndex(t *testing.T) {
	ctx := testkit.Context(t)

	r := &handler.CreateBulkRequest{Users: []*handler.CreateBulkItem{
		item("ok@example.com", "Fine"),
		item("not-an-email", "Second"),
	}}

	fields := fieldsOf(t, r.Valid(ctx))

	assert.Contains(t, fields, "users.1.email")
	assert.NotContains(t, fields, "users.0.email", "the valid entry is not reported")
}

// Unique only asks the database, which knows nothing about the rest of the
// payload — a repeat within one request has to be caught separately.
func TestCreateBulkRequest_rejectsDuplicateWithinTheRequest(t *testing.T) {
	ctx := testkit.Context(t)

	r := &handler.CreateBulkRequest{Users: []*handler.CreateBulkItem{
		item("dup@example.com", "First"),
		item("other@example.com", "Other"),
		item("DUP@example.com", "Second"), // same address, different case
	}}

	fields := fieldsOf(t, r.Valid(ctx))

	require.Contains(t, fields, "users.2.email", "the repeat is reported, not the original")
	assert.NotContains(t, fields, "users.0.email")
}

// An address already in the database is caught by Unique.
func TestCreateBulkRequest_rejectsExistingEmail(t *testing.T) {
	ctx := testkit.Context(t)
	require.NoError(t, ctx.DB().Create(&models.User{
		BaseModel: models.NewBaseModel(),
		Email:     "taken@example.com",
		FullName:  "Taken",
	}).Error)

	r := &handler.CreateBulkRequest{Users: []*handler.CreateBulkItem{item("taken@example.com", "Clash")}}

	assert.Contains(t, fieldsOf(t, r.Valid(ctx)), "users.0.email")
}

func TestCreateBulkRequest_rejectsOversizedBatch(t *testing.T) {
	ctx := testkit.Context(t)

	users := make([]*handler.CreateBulkItem, 0, 101)
	for i := range 101 {
		users = append(users, item(fmt.Sprintf("u%03d@example.com", i), "User"))
	}

	assert.Contains(t, fieldsOf(t, (&handler.CreateBulkRequest{Users: users}).Valid(ctx)), "users")
}
