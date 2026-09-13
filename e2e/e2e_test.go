//go:build e2e

// Package e2e drives a *running* service over the network — a compiled binary,
// the config it loaded itself, the database it migrated itself.
//
// Behind the `e2e` build tag so `make test` never compiles it: these need
// something deployed, and a suite that fails when nothing is running stops being
// useful feedback. Run them with `make test-e2e`.
//
// Keep this file small. Everything cheaper to test belongs in the packages that
// own the code — modules/user tests the same endpoints in-process, in
// milliseconds, and points at the failing line when it breaks. What only these
// can answer is "does the thing we deployed actually work".
package e2e_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/coretest"
	"github.com/pskclub/mine-core/v2/utils"
)

// ready returns a client once the service answers. Skips when E2E_BASE_URL is
// unset, so `go test --tags=e2e ./...` without a service up stays green.
func ready(t *testing.T) *coretest.Client {
	t.Helper()

	c := coretest.NewClientFromEnv(t)
	c.WaitReady("/healthz", 30*time.Second)

	return c
}

// signIn registers a throwaway account and returns a client carrying its token.
// The user routes are behind authentication, so this is the price of reaching
// them — and it exercises register and login on the deployed service for free.
func signIn(t *testing.T, c *coretest.Client) *coretest.Client {
	t.Helper()

	email := "e2e-" + utils.NewUUID()[:8] + "@example.com"
	const password = "correct-horse-battery"

	c.Post("/auth/register", map[string]any{
		"email":     email,
		"full_name": "E2E Runner",
		"password":  password,
	}).RequireStatus(http.StatusCreated)

	token, _ := c.Post("/auth/login", map[string]any{
		"email":    email,
		"password": password,
	}).RequireStatus(http.StatusOK).Map()["token"].(string)
	require.NotEmpty(t, token, "login must return a token")

	return coretest.NewClient(t, c.BaseURL(), coretest.WithHeader("Authorization", "Bearer "+token))
}

// The deployed service must refuse an unauthenticated call. Cheap, and it is the
// one thing you never want to discover in production.
func TestE2E_userRoutesRequireAuth(t *testing.T) {
	ready(t).Get("/users").RequireStatus(http.StatusUnauthorized)
}

// register, sign in, use the token, revoke it.
func TestE2E_authLifecycle(t *testing.T) {
	c := ready(t)
	authed := signIn(t, c)

	me := authed.Get("/auth/me").RequireStatus(http.StatusOK).Map()
	assert.NotEmpty(t, me["email"])
	assert.NotContains(t, me, "password", "the hash never leaves the server")

	authed.Post("/auth/logout", nil).RequireStatus(http.StatusNoContent)
	authed.Get("/auth/me").RequireStatus(http.StatusUnauthorized)
}

// The smoke test: the process is up, listening, and serving.
func TestE2E_health(t *testing.T) {
	c := ready(t)

	body := c.Get("/healthz").RequireStatus(http.StatusOK).Map()
	assert.Equal(t, core.HealthUp, body["status"])
}

// Readiness is the one that touches the dependencies, so it is the one that
// proves the deployment is wired — a 200 here means the database this instance
// was given actually answers.
func TestE2E_ready(t *testing.T) {
	c := ready(t)

	body := c.Get("/readyz").RequireStatus(http.StatusOK).Map()
	assert.Equal(t, core.HealthUp, body["status"])

	checks, ok := body["checks"].(map[string]any)
	require.True(t, ok, "readiness reports each dependency: %v", body)
	assert.Contains(t, checks, "database")
}

// One full journey through the deployed stack: create, read back, list, delete.
// It proves routing, binding, validation, the database connection and the
// migrations are all in place — the things that only exist once something is
// actually running.
func TestE2E_userLifecycle(t *testing.T) {
	c := signIn(t, ready(t))

	// unique per run: this talks to a shared database that is not reset between
	// runs, so fixed values would collide with yesterday's
	suffix := utils.NewUUID()[:8]
	email := "e2e-" + suffix + "@example.com"
	name := "E2E User " + suffix

	created := c.Post("/users", map[string]any{
		"email":     email,
		"full_name": name,
	}).RequireStatus(http.StatusCreated).Map()

	id, ok := created["id"].(string)
	require.True(t, ok, "the response must carry an id")

	// clean up after ourselves — this database outlives the test
	t.Cleanup(func() { c.Delete("/users/" + id) })

	t.Run("read back", func(t *testing.T) {
		body := c.Get("/users/" + id).RequireStatus(http.StatusOK).Map()
		assert.Equal(t, email, body["email"])
	})

	// q matches full_name — see repo.UserSearch for the fields it covers.
	t.Run("appears in the list", func(t *testing.T) {
		body := c.Get("/users?limit=10&page=1&q=" + suffix).RequireStatus(http.StatusOK).Map()
		assert.GreaterOrEqual(t, body["total"], float64(1))
	})

	t.Run("update", func(t *testing.T) {
		body := c.Put("/users/"+id, map[string]any{"full_name": "E2E Renamed " + suffix}).
			RequireStatus(http.StatusOK).Map()
		assert.Equal(t, "E2E Renamed "+suffix, body["full_name"])
	})

	t.Run("delete", func(t *testing.T) {
		c.Delete("/users/" + id).RequireStatus(http.StatusNoContent)
		c.Get("/users/" + id).RequireStatus(http.StatusNotFound)
	})
}

// Validation is worth one check here — not to re-test the rules, which the
// module's own tests cover, but to confirm the deployed service still renders
// errors in the shape clients parse.
func TestE2E_errorShape(t *testing.T) {
	c := signIn(t, ready(t))

	body := c.Post("/users", `{"email":"not-an-email"}`).
		RequireStatus(http.StatusBadRequest).
		Error()

	assert.Equal(t, "INVALID_PARAMS", body.Code)
	assert.Equal(t, "INVALID_EMAIL", body.Fields["email"].Code)
	assert.Equal(t, "body", body.Fields["email"].In, "field sources survive deployment")
}
