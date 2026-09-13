package auth_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pskclub/mine-core-template/models"
	"github.com/pskclub/mine-core-template/modules/auth/service"
	"github.com/pskclub/mine-core-template/testkit"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/coretest"
	"github.com/pskclub/mine-core/v2/repository"
)

const (
	testEmail    = "member@example.com"
	testPassword = "correct-horse"
)

// serve starts the whole service, wired by cmd.NewAPI — the same function
// cmd runs. These tests therefore cover the wiring as well as the handlers: an
// auth module registered without its guard, or with the wrong user service
// behind it, fails here rather than in production.
func serve(t *testing.T) (*coretest.Client, *core.App) {
	t.Helper()

	return testkit.Serve(t)
}

// register, then sign in, and return the token.
func registerAndLogin(t *testing.T, c *coretest.Client) string {
	t.Helper()

	c.Post("/auth/register", map[string]any{
		"email":     testEmail,
		"full_name": "Member",
		"password":  testPassword,
	}).RequireStatus(http.StatusCreated)

	token, _ := c.Post("/auth/login", map[string]any{
		"email":    testEmail,
		"password": testPassword,
	}).RequireStatus(http.StatusOK).Map()["token"].(string)

	require.NotEmpty(t, token)
	return token
}

func TestAuth_registerThenLogin(t *testing.T) {
	c, _ := serve(t)

	created := c.Post("/auth/register", map[string]any{
		"email":     testEmail,
		"full_name": "Member",
		"password":  testPassword,
	}).RequireStatus(http.StatusCreated).Map()

	assert.Equal(t, testEmail, created["email"])
	assert.NotContains(t, created, "password", "the hash must never be serialised")

	body := c.Post("/auth/login", map[string]any{
		"email":    testEmail,
		"password": testPassword,
	}).RequireStatus(http.StatusOK).Map()

	assert.NotEmpty(t, body["token"])
	assert.Equal(t, float64(service.TokenTTL.Seconds()), body["expires_in"])

	// flat: the user's fields sit alongside the token, not under a "user" key
	assert.NotContains(t, body, "user")
	assert.Equal(t, testEmail, body["email"])
	assert.Equal(t, "Member", body["full_name"])
	assert.NotEmpty(t, body["id"])
	assert.NotContains(t, body, "password", "the hash must never be serialised")
}

// Only the digest is stored, so a leaked database yields nothing usable.
func TestAuth_storesOnlyTheDigest(t *testing.T) {
	c, app := serve(t)
	token := registerAndLogin(t, c)

	ctx := app.NewContext(t.Context(), core.ModeTest)
	record, err := repository.New[models.AccessToken](ctx).
		Where("token_hash = ?", service.HashToken(token)).FindOne()

	require.Nil(t, err, "the row is found by the digest")
	assert.NotEqual(t, token, record.TokenHash, "the token itself is not stored")
	assert.Len(t, record.TokenHash, 64, "hex-encoded sha256")

	// and the token cannot be found by its plaintext
	_, err = repository.New[models.AccessToken](ctx).Where("token_hash = ?", token).FindOne()
	assert.NotNil(t, err)
}

// A wrong password and an unknown address answer identically, so this endpoint
// cannot be used to discover which addresses are registered.
func TestAuth_login_failsIdenticallyForBothMistakes(t *testing.T) {
	c, _ := serve(t)
	registerAndLogin(t, c)

	wrongPassword := c.Post("/auth/login", map[string]any{
		"email":    testEmail,
		"password": "not-the-password",
	}).RequireStatus(http.StatusUnauthorized).Error()

	unknownEmail := c.Post("/auth/login", map[string]any{
		"email":    "nobody@example.com",
		"password": testPassword,
	}).RequireStatus(http.StatusUnauthorized).Error()

	assert.Equal(t, "INVALID_CREDENTIALS", wrongPassword.Code)
	assert.Equal(t, wrongPassword.Code, unknownEmail.Code)
	assert.Equal(t, wrongPassword.Message, unknownEmail.Message)
}

func TestAuth_me(t *testing.T) {
	c, _ := serve(t)
	token := registerAndLogin(t, c)
	authed := coretest.NewClient(t, c.BaseURL(), coretest.WithHeader("Authorization", "Bearer "+token))

	body := authed.Get("/auth/me").RequireStatus(http.StatusOK).Map()

	assert.Equal(t, testEmail, body["email"])
	assert.NotContains(t, body, "password")
}

func TestAuth_me_rejectsMissingAndBadTokens(t *testing.T) {
	c, _ := serve(t)
	registerAndLogin(t, c)

	t.Run("no header", func(t *testing.T) {
		c.Get("/auth/me").RequireStatus(http.StatusUnauthorized)
	})

	t.Run("unknown token", func(t *testing.T) {
		bad := coretest.NewClient(t, c.BaseURL(), coretest.WithHeader("Authorization", "Bearer nope"))
		bad.Get("/auth/me").RequireStatus(http.StatusUnauthorized)
	})

	t.Run("not a bearer header", func(t *testing.T) {
		basic := coretest.NewClient(t, c.BaseURL(), coretest.WithHeader("Authorization", "Basic abc"))
		basic.Get("/auth/me").RequireStatus(http.StatusUnauthorized)
	})
}

// Revocation is the point of an opaque token: after logout it stops working,
// with nothing to wait for.
func TestAuth_logoutRevokesImmediately(t *testing.T) {
	c, _ := serve(t)
	token := registerAndLogin(t, c)
	authed := coretest.NewClient(t, c.BaseURL(), coretest.WithHeader("Authorization", "Bearer "+token))

	authed.Get("/auth/me").RequireStatus(http.StatusOK)
	authed.Post("/auth/logout", nil).RequireStatus(http.StatusNoContent)
	authed.Get("/auth/me").RequireStatus(http.StatusUnauthorized)
}

// An expired token is refused even though its row still exists.
func TestAuth_expiredTokenIsRejected(t *testing.T) {
	c, app := serve(t)
	token := registerAndLogin(t, c)

	ctx := app.NewContext(t.Context(), core.ModeTest)
	past := time.Now().UTC().Add(-time.Minute)
	require.Nil(t, repository.New[models.AccessToken](ctx).
		Where("token_hash = ?", service.HashToken(token)).
		Updates(map[string]any{"expires_at": past}))

	authed := coretest.NewClient(t, c.BaseURL(), coretest.WithHeader("Authorization", "Bearer "+token))

	// a distinct code: the token was real, it has simply run out, and the client
	// should sign in again rather than treat it as a bad token
	body := authed.Get("/auth/me").RequireStatus(http.StatusUnauthorized).Error()
	assert.Equal(t, "TOKEN_EXPIRED", body.Code)
}

// The stored expiry must be TokenTTL away as an *instant*, not as a wall clock.
// Writing a local time into a TIMESTAMP column drops the offset, so on postgres
// a 24h token silently became 31h in +07 — a difference sqlite cannot show.
func TestAuth_expiryIsStoredAsAnInstant(t *testing.T) {
	c, app := serve(t)
	token := registerAndLogin(t, c)

	ctx := app.NewContext(t.Context(), core.ModeTest)
	record, err := repository.New[models.AccessToken](ctx).
		Where("token_hash = ?", service.HashToken(token)).FindOne()
	require.Nil(t, err)

	assert.WithinDuration(t, time.Now().UTC().Add(service.TokenTTL), *record.ExpiresAt, time.Minute)
}

// Two sign-ins are two independent tokens: revoking one must not affect the other.
func TestAuth_tokensAreIndependent(t *testing.T) {
	c, _ := serve(t)
	first := registerAndLogin(t, c)

	second, _ := c.Post("/auth/login", map[string]any{
		"email":    testEmail,
		"password": testPassword,
	}).RequireStatus(http.StatusOK).Map()["token"].(string)
	require.NotEqual(t, first, second, "each sign-in issues a fresh token")

	firstClient := coretest.NewClient(t, c.BaseURL(), coretest.WithHeader("Authorization", "Bearer "+first))
	secondClient := coretest.NewClient(t, c.BaseURL(), coretest.WithHeader("Authorization", "Bearer "+second))

	firstClient.Post("/auth/logout", nil).RequireStatus(http.StatusNoContent)

	firstClient.Get("/auth/me").RequireStatus(http.StatusUnauthorized)
	secondClient.Get("/auth/me").RequireStatus(http.StatusOK)
}

func TestAuth_registerValidation(t *testing.T) {
	c, _ := serve(t)

	t.Run("rejects a short password and a bad address", func(t *testing.T) {
		codes := c.Post("/auth/register", map[string]any{
			"email":     "not-an-email",
			"full_name": "Member",
			"password":  "short",
		}).RequireStatus(http.StatusBadRequest).FieldCodes()

		assert.Equal(t, "INVALID_EMAIL", codes["email"])
		assert.Equal(t, "INVALID_STRING_LENGTH", codes["password"])
	})

	t.Run("rejects an address already registered", func(t *testing.T) {
		registerAndLogin(t, c)

		codes := c.Post("/auth/register", map[string]any{
			"email":     testEmail,
			"full_name": "Impostor",
			"password":  testPassword,
		}).RequireStatus(http.StatusBadRequest).FieldCodes()

		assert.Contains(t, codes, "email")
	})
}
