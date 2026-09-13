package service_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pskclub/mine-core-template/models"
	"github.com/pskclub/mine-core-template/modules/auth/service"
	"github.com/pskclub/mine-core-template/modules/user"
	"github.com/pskclub/mine-core-template/testkit"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/utils"
)

const (
	authEmail    = "member@example.com"
	authPassword = "correct-horse"
)

// usersFor is the dependency cmd.NewAPI supplies in production, repeated
// here so these tests exercise the real user service rather than a stub. A stub
// would let a change to how accounts are stored pass unnoticed, and the point of
// these tests is that registering and signing in agree about that.
//
// The import is legal only because this is an external test package: modules/auth
// itself must never import modules/user, or the two would not compile.
func usersFor(ctx core.IContext) service.Users { return user.NewUserService(ctx) }

func newSvc(ctx core.IContext) service.IAuthService {
	return service.NewAuthService(ctx, usersFor(ctx))
}

// register puts an account in place for the tests that need one to sign in with.
// It goes through the service rather than testkit.SeedUsers because only Register
// writes a password the sign-in path can match — a seeded user stores a literal,
// which is not a bcrypt hash and can never verify.
func register(t *testing.T, ctx core.IContext) *models.User {
	t.Helper()

	u, err := newSvc(ctx).Register(&service.RegisterPayload{
		Email:    authEmail,
		FullName: "Member",
		Password: authPassword,
	})
	require.Nil(t, err)

	return u
}

// login registers an account and signs in, returning the token.
func login(t *testing.T, ctx core.IContext) *service.AuthResult {
	t.Helper()

	register(t, ctx)
	result, err := newSvc(ctx).Login(&service.LoginPayload{
		Email:    authEmail,
		Password: authPassword,
	})
	require.Nil(t, err)

	return result
}

func TestAuthService_Register(t *testing.T) {
	ctx := testkit.Context(t)

	u := register(t, ctx)

	assert.NotEmpty(t, u.ID, "the service assigns the id")
	assert.Equal(t, authEmail, u.Email)
	assert.Equal(t, "Member", u.FullName)

	// What lands in the row is the hash, not the password. Read it back rather
	// than trusting the returned struct: this is the assertion that a leaked
	// database yields nothing to sign in with.
	var row models.User
	require.NoError(t, ctx.DB().Where("id = ?", u.ID).First(&row).Error,
		"the user reached the database")
	assert.NotEqual(t, authPassword, row.Password, "the plaintext is never stored")
	assert.True(t, utils.ComparePassword(row.Password, authPassword),
		"and what is stored verifies the password")
}

// A second account on the same address is a database failure, not a silent
// overwrite. The unique index is what enforces it — sqlite's autoMigrate schema
// has no partial index, so the test creates the one the migration deploys.
func TestAuthService_Register_duplicateEmail(t *testing.T) {
	ctx := testkit.Context(t)
	require.NoError(t, ctx.DB().Exec(
		`CREATE UNIQUE INDEX idx_users_email ON users(email) WHERE deleted_at IS NULL`).Error)

	register(t, ctx)

	_, err := newSvc(ctx).Register(&service.RegisterPayload{
		Email:    authEmail,
		FullName: "Impostor",
		Password: authPassword,
	})

	require.NotNil(t, err)
	assert.GreaterOrEqual(t, err.GetStatus(), http.StatusInternalServerError)
}

func TestAuthService_Login(t *testing.T) {
	ctx := testkit.Context(t)
	u := register(t, ctx)

	result, err := newSvc(ctx).Login(&service.LoginPayload{
		Email:    authEmail,
		Password: authPassword,
	})

	require.Nil(t, err)
	require.NotNil(t, result.User, "the user is embedded, and marshalling a nil one panics")
	assert.Equal(t, u.ID, result.User.ID)
	assert.NotEmpty(t, result.Token)
	assert.Equal(t, int64(service.TokenTTL.Seconds()), result.ExpiresIn)
}

// Only the digest is stored, so the token cannot be read back out of the table.
func TestAuthService_Login_storesOnlyTheDigest(t *testing.T) {
	ctx := testkit.Context(t)
	result := login(t, ctx)

	var row models.AccessToken
	require.NoError(t, ctx.DB().
		Where("token_hash = ?", service.HashToken(result.Token)).First(&row).Error)

	assert.Equal(t, result.User.ID, row.UserID)
	assert.NotEqual(t, result.Token, row.TokenHash, "the token itself is not stored")
	assert.Len(t, row.TokenHash, 64, "hex-encoded sha256")

	// and nothing matches the plaintext
	var count int64
	require.NoError(t, ctx.DB().Model(&models.AccessToken{}).
		Where("token_hash = ?", result.Token).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

// The expiry is an instant TokenTTL away, not a wall clock. Writing a local time
// into a TIMESTAMP column drops the offset, so a 24h token silently becomes 31h
// in +07 — a difference only the postgres run can show, which is why the
// assertion lives here rather than in the reading of the struct.
func TestAuthService_Login_expiryIsStoredAsAnInstant(t *testing.T) {
	ctx := testkit.Context(t)
	result := login(t, ctx)

	var row models.AccessToken
	require.NoError(t, ctx.DB().
		Where("token_hash = ?", service.HashToken(result.Token)).First(&row).Error)

	require.NotNil(t, row.ExpiresAt)
	assert.WithinDuration(t, time.Now().UTC().Add(service.TokenTTL), *row.ExpiresAt, time.Minute)
}

// A wrong password and an unknown address answer identically, so sign-in cannot
// be used to discover which addresses are registered.
func TestAuthService_Login_failsIdenticallyForBothMistakes(t *testing.T) {
	ctx := testkit.Context(t)
	register(t, ctx)
	svc := newSvc(ctx)

	_, wrongPassword := svc.Login(&service.LoginPayload{
		Email:    authEmail,
		Password: "not-the-password",
	})
	_, unknownEmail := svc.Login(&service.LoginPayload{
		Email:    "nobody@example.com",
		Password: authPassword,
	})

	require.NotNil(t, wrongPassword)
	require.NotNil(t, unknownEmail)
	assert.Equal(t, http.StatusUnauthorized, wrongPassword.GetStatus())
	assert.Equal(t, "INVALID_CREDENTIALS", wrongPassword.GetCode())
	assert.Equal(t, wrongPassword.GetCode(), unknownEmail.GetCode())
	assert.Equal(t, wrongPassword.GetMessage(), unknownEmail.GetMessage())

	// a failed attempt issues nothing
	var count int64
	require.NoError(t, ctx.DB().Model(&models.AccessToken{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

// Two sign-ins are two independent tokens: one must not stand in for the other.
func TestAuthService_Login_issuesAFreshTokenEachTime(t *testing.T) {
	ctx := testkit.Context(t)
	first := login(t, ctx)

	second, err := newSvc(ctx).Login(&service.LoginPayload{
		Email:    authEmail,
		Password: authPassword,
	})

	require.Nil(t, err)
	assert.NotEqual(t, first.Token, second.Token)

	var count int64
	require.NoError(t, ctx.DB().Model(&models.AccessToken{}).Count(&count).Error)
	assert.Equal(t, int64(2), count, "both sessions are live at once")
}

// Revocation is the point of an opaque token: after logout the digest no longer
// resolves, with nothing to wait for.
func TestAuthService_Logout(t *testing.T) {
	ctx := testkit.Context(t)
	result := login(t, ctx)

	require.Nil(t, newSvc(ctx).Logout(result.Token))

	var count int64
	require.NoError(t, ctx.DB().Model(&models.AccessToken{}).
		Where("token_hash = ?", service.HashToken(result.Token)).Count(&count).Error)
	assert.Equal(t, int64(0), count, "the token no longer resolves")

	// The delete is soft, so the row survives — and with it the unique digest,
	// which is harmless because a token is 256 bits of randomness.
	require.NoError(t, ctx.DB().Unscoped().Model(&models.AccessToken{}).
		Where("token_hash = ?", service.HashToken(result.Token)).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

// The caller wanted to be signed out, and they are — whether or not the token
// was ever real.
func TestAuthService_Logout_unknownAndEmptyTokens(t *testing.T) {
	ctx := testkit.Context(t)
	live := login(t, ctx)
	svc := newSvc(ctx)

	t.Run("unknown token", func(t *testing.T) {
		assert.Nil(t, svc.Logout("nope"))
	})

	t.Run("empty token", func(t *testing.T) {
		assert.Nil(t, svc.Logout(""))
	})

	// and neither took the live session down with it
	var count int64
	require.NoError(t, ctx.DB().Model(&models.AccessToken{}).
		Where("token_hash = ?", service.HashToken(live.Token)).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

// Logging one session out leaves the other alone.
func TestAuthService_Logout_leavesOtherSessions(t *testing.T) {
	ctx := testkit.Context(t)
	first := login(t, ctx)
	svc := newSvc(ctx)

	second, err := svc.Login(&service.LoginPayload{Email: authEmail, Password: authPassword})
	require.Nil(t, err)

	require.Nil(t, svc.Logout(first.Token))

	var count int64
	require.NoError(t, ctx.DB().Model(&models.AccessToken{}).
		Where("token_hash = ?", service.HashToken(second.Token)).Count(&count).Error)
	assert.Equal(t, int64(1), count, "the other session is untouched")
}

func TestAuthService_Me(t *testing.T) {
	ctx := testkit.Context(t)
	u := register(t, ctx)
	ctx.SetUser(&core.ContextUser{ID: u.ID, Email: u.Email, Name: u.FullName})

	me, err := newSvc(ctx).Me()

	require.Nil(t, err)
	assert.Equal(t, u.ID, me.ID)
	assert.Equal(t, authEmail, me.Email)
}

// The row is read fresh rather than taken from the token, so a change to the
// account shows up on the next request instead of after the token expires.
func TestAuthService_Me_readsTheRowNotTheToken(t *testing.T) {
	ctx := testkit.Context(t)
	u := register(t, ctx)
	// the token still names the old one
	ctx.SetUser(&core.ContextUser{ID: u.ID, Email: u.Email, Name: "Member"})

	require.NoError(t, ctx.DB().Model(&models.User{}).
		Where("id = ?", u.ID).Update("full_name", "Member Renamed").Error)

	me, err := newSvc(ctx).Me()

	require.Nil(t, err)
	assert.Equal(t, "Member Renamed", me.FullName)
}

func TestAuthService_Me_withoutASignedInUser(t *testing.T) {
	svc := newSvc(testkit.Context(t))

	t.Run("no user on the context", func(t *testing.T) {
		_, err := svc.Me()

		require.NotNil(t, err)
		assert.Equal(t, http.StatusUnauthorized, err.GetStatus())
		assert.Equal(t, "UNAUTHORIZED", err.GetCode())
	})

	t.Run("a user with no id", func(t *testing.T) {
		ctx := testkit.Context(t)
		ctx.SetUser(&core.ContextUser{})

		_, err := newSvc(ctx).Me()

		require.NotNil(t, err)
		assert.Equal(t, http.StatusUnauthorized, err.GetStatus())
	})
}

// A well-formed token naming an account that no longer exists is a rejection,
// not a 500: nothing is broken, the user is simply gone.
func TestAuthService_Me_deletedAccount(t *testing.T) {
	ctx := testkit.Context(t)
	u := register(t, ctx)
	ctx.SetUser(&core.ContextUser{ID: u.ID, Email: u.Email})

	require.NoError(t, ctx.DB().Where("id = ?", u.ID).Delete(&models.User{}).Error)

	_, err := newSvc(ctx).Me()

	require.NotNil(t, err)
	assert.Equal(t, http.StatusUnauthorized, err.GetStatus())
	assert.Equal(t, "UNAUTHORIZED", err.GetCode())
}

// ResolveToken is the lookup behind the guard and runs on every authenticated
// request, so it is tested against a real database rather than only through the
// HTTP layer.
//
// It takes an App because the guard runs before any handler — the request's
// context arrives per call — which is why these tests build one instead of using
// testkit.Context.
func TestResolveToken(t *testing.T) {
	app := testkit.App(t)
	ctx := app.NewContext(t.Context(), core.ModeTest)

	u := register(t, ctx)
	result, loginErr := newSvc(ctx).Login(&service.LoginPayload{
		Email:    authEmail,
		Password: authPassword,
	})
	require.Nil(t, loginErr)

	resolve := service.ResolveToken(app, usersFor)

	t.Run("a live token resolves to its user", func(t *testing.T) {
		resolved, err := resolve(t.Context(), service.HashToken(result.Token))

		require.NoError(t, err)
		assert.Equal(t, u.ID, resolved.ID)
		assert.Equal(t, authEmail, resolved.Email)
		assert.Equal(t, "Member", resolved.Name)
	})

	// The guard hands over a digest, so the plaintext must not resolve —
	// otherwise a leaked database would be as good as a token.
	t.Run("the plaintext token does not resolve", func(t *testing.T) {
		_, err := resolve(t.Context(), result.Token)

		require.Error(t, err)
		assert.Equal(t, errmsgsUnauthorizedCode, codeOf(t, err))
	})

	t.Run("an unknown token is refused", func(t *testing.T) {
		_, err := resolve(t.Context(), service.HashToken("never issued"))

		require.Error(t, err)
		assert.Equal(t, errmsgsUnauthorizedCode, codeOf(t, err))
	})
}

// An expired token has a row and still fails, with a code the client can act on:
// the token was real, it has simply run out, and the answer is to sign in again.
func TestResolveToken_expired(t *testing.T) {
	app := testkit.App(t)
	ctx := app.NewContext(t.Context(), core.ModeTest)

	register(t, ctx)
	result, loginErr := newSvc(ctx).Login(&service.LoginPayload{
		Email:    authEmail,
		Password: authPassword,
	})
	require.Nil(t, loginErr)

	past := time.Now().UTC().Add(-time.Minute)
	require.NoError(t, ctx.DB().Model(&models.AccessToken{}).
		Where("token_hash = ?", service.HashToken(result.Token)).
		Update("expires_at", past).Error)

	_, err := service.ResolveToken(app, usersFor)(t.Context(), service.HashToken(result.Token))

	require.Error(t, err)
	assert.Equal(t, "TOKEN_EXPIRED", codeOf(t, err))
}

// A token outlives the account it names: the row is gone, so the token is no
// longer anybody's.
func TestResolveToken_deletedAccount(t *testing.T) {
	app := testkit.App(t)
	ctx := app.NewContext(t.Context(), core.ModeTest)

	u := register(t, ctx)
	result, loginErr := newSvc(ctx).Login(&service.LoginPayload{
		Email:    authEmail,
		Password: authPassword,
	})
	require.Nil(t, loginErr)

	require.NoError(t, ctx.DB().Where("id = ?", u.ID).Delete(&models.User{}).Error)

	_, err := service.ResolveToken(app, usersFor)(t.Context(), service.HashToken(result.Token))

	require.Error(t, err)
	assert.Equal(t, errmsgsUnauthorizedCode, codeOf(t, err))
}

// A logged-out token stops resolving immediately — the whole reason the token is
// opaque rather than self-contained.
func TestResolveToken_afterLogout(t *testing.T) {
	app := testkit.App(t)
	ctx := app.NewContext(t.Context(), core.ModeTest)

	register(t, ctx)
	svc := newSvc(ctx)
	result, loginErr := svc.Login(&service.LoginPayload{Email: authEmail, Password: authPassword})
	require.Nil(t, loginErr)

	require.Nil(t, svc.Logout(result.Token))

	_, err := service.ResolveToken(app, usersFor)(t.Context(), service.HashToken(result.Token))

	require.Error(t, err)
	assert.Equal(t, errmsgsUnauthorizedCode, codeOf(t, err))
}

// The digest is stable and one-way, which is what lets Login's write and the
// guard's lookup agree without either holding the token.
func TestHashToken(t *testing.T) {
	first := service.HashToken("a-token")

	assert.Equal(t, first, service.HashToken("a-token"), "the same token, the same digest")
	assert.NotEqual(t, first, service.HashToken("b-token"))
	assert.NotEqual(t, "a-token", first)
	assert.Len(t, first, 64, "hex-encoded sha256")
}

const errmsgsUnauthorizedCode = "UNAUTHORIZED"

// codeOf reads the framework's error code off the error ResolveToken returns. It
// is declared as a plain error — the guard's signature — but carries an IError,
// which is how a distinct code survives to the client.
func codeOf(t *testing.T, err error) string {
	t.Helper()

	ierr, ok := err.(core.IError)
	require.True(t, ok, "the guard passes an IError through untouched")

	return ierr.GetCode()
}
