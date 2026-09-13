package note_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pskclub/mine-core-template/testkit"
	"github.com/pskclub/mine-core/v2/coretest"
)

// serve starts the real service and returns a signed-in client.
func serve(t *testing.T) *coretest.Client {
	t.Helper()

	anon, _ := testkit.Serve(t)

	return testkit.SignIn(t, anon)
}

// signInAs registers a second account and returns a client for it, for the tests
// that need two callers to prove one cannot see the other's rows.
func signInAs(t *testing.T, c *coretest.Client, email string) *coretest.Client {
	t.Helper()

	c.Post("/auth/register", map[string]any{
		"email":     email,
		"full_name": "Other",
		"password":  testkit.FixturePassword,
	}).RequireStatus(http.StatusCreated)

	token, _ := c.Post("/auth/login", map[string]any{
		"email":    email,
		"password": testkit.FixturePassword,
	}).RequireStatus(http.StatusOK).Map()["token"].(string)
	require.NotEmpty(t, token)

	return coretest.NewClient(t, c.BaseURL(),
		coretest.WithHeader("Authorization", "Bearer "+token))
}

func create(t *testing.T, c *coretest.Client, title string) string {
	t.Helper()

	id, _ := c.Post("/notes", map[string]any{
		"title": title,
		"body":  "written by a test",
	}).RequireStatus(http.StatusCreated).Map()["id"].(string)
	require.NotEmpty(t, id)

	return id
}

func TestNoteAPI_requiresAuthentication(t *testing.T) {
	c := serve(t)
	anon := coretest.NewClient(t, c.BaseURL())

	anon.Get("/notes").RequireStatus(http.StatusUnauthorized)
	anon.Post("/notes", map[string]any{"title": "x", "body": "y"}).
		RequireStatus(http.StatusUnauthorized)
	anon.Get("/notes/00000000-0000-0000-0000-000000000000").
		RequireStatus(http.StatusUnauthorized)
	anon.Put("/notes/00000000-0000-0000-0000-000000000000", map[string]any{"title": "x", "body": "y"}).
		RequireStatus(http.StatusUnauthorized)
	anon.Delete("/notes/00000000-0000-0000-0000-000000000000").
		RequireStatus(http.StatusUnauthorized)
}

func TestNoteAPI_crud(t *testing.T) {
	c := serve(t)

	created := c.Post("/notes", map[string]any{
		"title":  "First",
		"body":   "hello",
		"pinned": true,
	}).RequireStatus(http.StatusCreated).Map()

	id, _ := created["id"].(string)
	require.NotEmpty(t, id)
	assert.Equal(t, "First", created["title"])
	assert.Equal(t, true, created["pinned"])

	// the owner is taken from the token, not from the body — a client cannot
	// write a note into somebody else's account by sending a user_id
	assert.NotEmpty(t, created["user_id"])

	t.Run("find", func(t *testing.T) {
		body := c.Get("/notes/" + id).RequireStatus(http.StatusOK).Map()
		assert.Equal(t, "hello", body["body"])
	})

	t.Run("update", func(t *testing.T) {
		body := c.Put("/notes/"+id, map[string]any{
			"title":  "First, edited",
			"body":   "hello again",
			"pinned": false,
		}).RequireStatus(http.StatusOK).Map()

		assert.Equal(t, "First, edited", body["title"])
		assert.Equal(t, false, body["pinned"])
	})

	t.Run("delete", func(t *testing.T) {
		c.Delete("/notes/" + id).RequireStatus(http.StatusNoContent)
		c.Get("/notes/" + id).RequireStatus(http.StatusNotFound)
	})
}

// The rule this module exists to demonstrate: a note belongs to one account, and
// every other account is answered as though it does not exist.
func TestNoteAPI_oneUserCannotReachAnothersNotes(t *testing.T) {
	mine := serve(t)
	theirs := signInAs(t, mine, "other@example.com")

	id := create(t, mine, "Private")

	t.Run("not in their list", func(t *testing.T) {
		body := theirs.Get("/notes?limit=10&page=1").RequireStatus(http.StatusOK).Map()
		assert.Equal(t, float64(0), body["total"])
	})

	// 404 and not 403: a 403 would confirm the id names a real note, which is a
	// fact the caller has no right to.
	t.Run("reading it is a 404", func(t *testing.T) {
		res := theirs.Get("/notes/" + id).RequireStatus(http.StatusNotFound)
		assert.Equal(t, "NOT_FOUND", res.Error().Code)
	})

	t.Run("editing it is a 404", func(t *testing.T) {
		theirs.Put("/notes/"+id, map[string]any{"title": "Hijacked", "body": "x"}).
			RequireStatus(http.StatusNotFound)
	})

	t.Run("deleting it is a 404", func(t *testing.T) {
		theirs.Delete("/notes/" + id).RequireStatus(http.StatusNotFound)
	})

	// and none of that touched it
	body := mine.Get("/notes/" + id).RequireStatus(http.StatusOK).Map()
	assert.Equal(t, "Private", body["title"])
}

func TestNoteAPI_listIsScopedSortedAndSearchable(t *testing.T) {
	c := serve(t)

	create(t, c, "Groceries")
	create(t, c, "Reading list")
	pinned := create(t, c, "Pinned one")
	c.Put("/notes/"+pinned, map[string]any{
		"title": "Pinned one", "body": "x", "pinned": true,
	}).RequireStatus(http.StatusOK)

	t.Run("pinned notes come first", func(t *testing.T) {
		body := c.Get("/notes?limit=10&page=1").RequireStatus(http.StatusOK).Map()

		assert.Equal(t, float64(3), body["total"])
		items, _ := body["items"].([]any)
		require.NotEmpty(t, items)
		first, _ := items[0].(map[string]any)
		assert.Equal(t, "Pinned one", first["title"], "the default order puts pinned first")
	})

	t.Run("q matches the title", func(t *testing.T) {
		body := c.Get("/notes?limit=10&page=1&q=grocer").RequireStatus(http.StatusOK).Map()
		assert.Equal(t, float64(1), body["total"])
	})
}

func TestNoteAPI_validation(t *testing.T) {
	c := serve(t)

	t.Run("title and body are required", func(t *testing.T) {
		codes := c.Post("/notes", `{}`).RequireStatus(http.StatusBadRequest).FieldCodes()

		assert.Equal(t, "REQUIRED", codes["title"])
		assert.Equal(t, "REQUIRED", codes["body"])
	})

	t.Run("pinned is not required — a missing one means false", func(t *testing.T) {
		body := c.Post("/notes", map[string]any{"title": "No flag", "body": "x"}).
			RequireStatus(http.StatusCreated).Map()

		assert.Equal(t, false, body["pinned"])
	})

	t.Run("the path id is validated with the body", func(t *testing.T) {
		fields := c.Put("/notes/not-a-uuid", `{"title":"x","body":"y"}`).
			RequireStatus(http.StatusBadRequest).Error().Fields

		assert.Equal(t, "INVALID_UUID", fields["id"].Code)
		assert.Equal(t, "path", fields["id"].In)
	})
}
