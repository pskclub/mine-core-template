package user_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pskclub/mine-core-template/models"
	"github.com/pskclub/mine-core-template/testkit"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/coretest"
	"github.com/pskclub/mine-core/v2/repository"
)

// serve starts the real service and returns a signed-in client.
//
// The whole service, not this module's routes alone: the guard on these routes
// comes from the auth module and is supplied by cmd.NewAPI, so a test that
// mounted this Module by hand would be testing a wiring that does not ship.
func serve(t *testing.T) (*coretest.Client, *core.App) {
	t.Helper()

	anon, app := testkit.Serve(t)

	return testkit.SignIn(t, anon), app
}

// Without a token the routes are closed. Worth its own test: g.Required() is
// written on every route by hand, and this is what proves none of them was
// missed.
func TestUserAPI_requiresAuthentication(t *testing.T) {
	c, _ := serve(t)
	anon := coretest.NewClient(t, c.BaseURL())

	anon.Get("/users").RequireStatus(http.StatusUnauthorized)
	anon.Post("/users", map[string]any{"email": "x@y.co", "full_name": "X"}).
		RequireStatus(http.StatusUnauthorized)
	anon.Get("/users/00000000-0000-0000-0000-000000000000").
		RequireStatus(http.StatusUnauthorized)
	anon.Post("/users/bulk", map[string]any{"users": []map[string]any{}}).
		RequireStatus(http.StatusUnauthorized)
	anon.Put("/users/00000000-0000-0000-0000-000000000000", map[string]any{"full_name": "X"}).
		RequireStatus(http.StatusUnauthorized)
	anon.Delete("/users/00000000-0000-0000-0000-000000000000").
		RequireStatus(http.StatusUnauthorized)
}

func TestUserAPI_rejectsAnUnknownToken(t *testing.T) {
	c, _ := serve(t)
	bogus := coretest.NewClient(t, c.BaseURL(),
		coretest.WithHeader("Authorization", "Bearer "+strings.Repeat("a", 64)))

	bogus.Get("/users").RequireStatus(http.StatusUnauthorized)
}

func TestUserAPI_create(t *testing.T) {
	c, _ := serve(t)

	body := c.Post("/users", map[string]any{
		"email":     "alice@example.com",
		"full_name": "Alice",
	}).RequireStatus(http.StatusCreated).Map()

	assert.Equal(t, "alice@example.com", body["email"])
	assert.NotEmpty(t, body["id"])
}

// Every violation is reported at once, and each says which part of the request
// it came from.
func TestUserAPI_create_validation(t *testing.T) {
	c, _ := serve(t)

	res := c.Post("/users", `{"email":"not-an-email","full_name":"x"}`).
		RequireStatus(http.StatusBadRequest)

	assert.Equal(t, map[string]string{
		"email":     "INVALID_EMAIL",
		"full_name": "INVALID_STRING_LENGTH",
	}, res.FieldCodes())

	assert.Equal(t, "body", res.Error().Fields["email"].In)
}

// The path parameter is bound and validated like any other field, so a bad id
// is a 400 naming "id" rather than a failed lookup.
func TestUserAPI_update_reportsWhereEachFieldCameFrom(t *testing.T) {
	c, _ := serve(t)

	fields := c.Put("/users/not-a-uuid", `{"full_name":"x"}`).
		RequireStatus(http.StatusBadRequest).
		Error().Fields

	assert.Equal(t, "INVALID_UUID", fields["id"].Code)
	assert.Equal(t, "path", fields["id"].In)
	assert.Equal(t, "body", fields["full_name"].In)
}

func TestUserAPI_bulkCreate(t *testing.T) {
	c, _ := serve(t)

	body := c.Post("/users/bulk", map[string]any{
		"users": []map[string]any{
			{"email": "one@example.com", "full_name": "One"},
			{"email": "two@example.com", "full_name": "Two"},
		},
	}).RequireStatus(http.StatusCreated).Map()

	assert.Equal(t, float64(2), body["count"])
}

// An address repeated inside one request is rejected before it reaches the
// database, and the violation names the entry to fix.
func TestUserAPI_bulkCreate_duplicateWithinRequest(t *testing.T) {
	c, _ := serve(t)

	fields := c.Post("/users/bulk", map[string]any{
		"users": []map[string]any{
			{"email": "dup@example.com", "full_name": "First"},
			{"email": "DUP@example.com", "full_name": "Second"},
		},
	}).RequireStatus(http.StatusBadRequest).Error().Fields

	assert.Equal(t, "DUPLICATE_IN_REQUEST", fields["users.1.email"].Code)
	assert.NotContains(t, fields, "users.0.email", "the first occurrence is not the problem")
}

// /users/bulk must win over /users/:id — a router that matched the parameter
// first would send "bulk" to Find and return 400 for a valid batch.
func TestUserAPI_bulkRouteIsNotShadowedByTheIDRoute(t *testing.T) {
	c, _ := serve(t)

	c.Post("/users/bulk", map[string]any{
		"users": []map[string]any{{"email": "route@example.com", "full_name": "Route"}},
	}).RequireStatus(http.StatusCreated)
}

func TestUserAPI_listAndFind(t *testing.T) {
	c, app := serve(t)

	ctx := app.NewContext(t.Context(), core.ModeTest)
	seeded := models.User{BaseModel: models.NewBaseModel(), Email: "seed@example.com", FullName: "Seeded", Password: "x"}
	require.Nil(t, repository.New[models.User](ctx).Create(&seeded))

	// Filtered rather than counted: the fixture signs in, which creates a user of
	// its own, so a bare total would assert on the fixture as much as the seed.
	t.Run("list", func(t *testing.T) {
		body := c.Get("/users?limit=10&page=1&q=Seeded").RequireStatus(http.StatusOK).Map()
		assert.Equal(t, float64(1), body["total"])
	})

	t.Run("find", func(t *testing.T) {
		body := c.Get("/users/" + seeded.ID).RequireStatus(http.StatusOK).Map()
		assert.Equal(t, "seed@example.com", body["email"])
	})

	t.Run("missing is 404", func(t *testing.T) {
		res := c.Get("/users/00000000-0000-0000-0000-000000000000").
			RequireStatus(http.StatusNotFound)
		assert.Equal(t, "NOT_FOUND", res.Error().Code)
	})
}

func TestUserAPI_delete(t *testing.T) {
	c, app := serve(t)

	ctx := app.NewContext(t.Context(), core.ModeTest)
	seeded := models.User{BaseModel: models.NewBaseModel(), Email: "gone@example.com", FullName: "Gone", Password: "x"}
	require.Nil(t, repository.New[models.User](ctx).Create(&seeded))

	c.Delete("/users/" + seeded.ID).RequireStatus(http.StatusNoContent)
	c.Get("/users/" + seeded.ID).RequireStatus(http.StatusNotFound)
}
