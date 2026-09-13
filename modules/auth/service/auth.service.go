// Package service holds the auth module's business rules: registering, signing
// in, revoking, and resolving a token to the user it names.
//
// It is imported by this module's handler and by modules/auth itself, and by
// nothing else — another module reaches auth through modules/auth.
package service

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/pskclub/mine-core-template/emsgs"
	"github.com/pskclub/mine-core-template/models"
	"github.com/pskclub/mine-core-template/modules/auth/store"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/errmsgs"
	"github.com/pskclub/mine-core/v2/utils"
)

const (
	// TokenTTL is how long an issued token stays valid. Short enough that a
	// leaked token expires on its own; long enough not to interrupt a session.
	TokenTTL = 24 * time.Hour

	// tokenBytes is the size of the random part. 32 bytes is 256 bits — far
	// beyond guessing, and the same width as the digest it is stored under.
	tokenBytes = 32
)

type IAuthService interface {
	Register(input *RegisterPayload) (*models.User, core.IError)
	Login(input *LoginPayload) (*AuthResult, core.IError)
	Logout(token string) core.IError
	Me() (*models.User, core.IError)
}

type authService struct {
	ctx   core.IContext
	users Users
}

// NewAuthService takes the user service alongside the context: auth does not own
// the users table, so every account read and write goes through the module that
// does. See auth.deps.go for why that is an interface rather than an import.
//
// No logger is stored: ctx.Log() already carries the request id, so a sign-in
// line and the request line that produced it join on one field without anything
// being held or passed.
func NewAuthService(ctx core.IContext, users Users) IAuthService {
	return &authService{ctx: ctx, users: users}
}

// Register hashes the password here rather than in the user module. Credential
// policy — which algorithm, what cost — is auth's; the user module stores the
// result and never sees a plaintext password at all.
func (s authService) Register(input *RegisterPayload) (*models.User, core.IError) {
	hash, err := utils.HashPassword(input.Password)
	if err != nil {
		return nil, s.ctx.NewError(err, errmsgs.InternalServerError)
	}

	user, createErr := s.users.CreateAccount(input.Email, input.FullName, hash)
	if createErr != nil {
		return nil, createErr
	}

	// The user module logs that a row was written; this logs that an account was
	// registered, which is a different fact and the one a security review reads.
	// Both carry the same request id, so they join.
	s.ctx.Log().Info("account registered", "user_id", user.ID)

	return user, nil
}

func (s authService) Login(input *LoginPayload) (*AuthResult, core.IError) {
	user, findErr := s.users.FindByEmail(input.Email)

	// One answer for both "no such account" and "wrong password". Telling them
	// apart lets anyone enumerate which addresses are registered, and the caller
	// can do nothing useful with the difference.
	if findErr != nil {
		if errmsgs.IsNotFoundError(findErr) {
			// The log records the distinction the response deliberately hides.
			// The client must not learn which addresses are registered; whoever
			// is investigating a burst of failures must, and they are reading a
			// log that never leaves the service.
			//
			// Warn, not Error: nothing is broken. One of these is somebody
			// mistyping, a thousand of them is an attack, and Warn is the level
			// that can be alerted on by rate without drowning the error channel.
			s.ctx.Log().Warn("sign-in failed", "reason", "unknown_account")

			return nil, emsgs.InvalidCredentials
		}
		return nil, s.ctx.NewError(findErr, findErr)
	}

	if !utils.ComparePassword(user.Password, input.Password) {
		// The user id is known here and the previous branch has none — which is
		// exactly what makes the two lines worth telling apart: this one says
		// whose account is being guessed at.
		s.ctx.Log().Warn("sign-in failed", "reason", "wrong_password", "user_id", user.ID)

		return nil, emsgs.InvalidCredentials
	}

	token, err := newToken()
	if err != nil {
		return nil, s.ctx.NewError(err, errmsgs.InternalServerError)
	}

	// UTC, not local. expires_at is TIMESTAMP without time zone, so the driver
	// writes the wall clock and discards the offset — a local time read back is
	// reinterpreted as UTC and silently jumps by that offset. In +07 a 24h token
	// would last 31. Core sets GORM's NowFunc to UTC for created_at/updated_at
	// for the same reason; a column we set ourselves has to do it too.
	expires := time.Now().UTC().Add(TokenTTL)
	record := &models.AccessToken{
		BaseModel: models.NewBaseModel(),
		UserID:    user.ID,
		TokenHash: HashToken(token), // only the digest is stored
		ExpiresAt: &expires,
	}
	if err := store.AccessToken(s.ctx).Create(record); err != nil {
		return nil, s.ctx.NewError(err, err)
	}

	// No token, and no digest either: a log store is read by more people than
	// the database is, and either value in a line is a credential in a place
	// that was never designed to hold one.
	s.ctx.Log().Info("signed in", "user_id", user.ID, "expires_in", int64(TokenTTL.Seconds()))

	// the only time the caller ever sees the token itself
	return &AuthResult{
		User:      user,
		Token:     token,
		ExpiresIn: int64(TokenTTL.Seconds()),
	}, nil
}

// Logout deletes the token's row, so it stops working immediately. This is the
// thing an opaque token buys over a self-contained one: revocation costs a
// DELETE rather than a blocklist every request has to consult.
//
// An unknown token is not an error — the caller wanted to be signed out, and
// they are.
func (s authService) Logout(token string) core.IError {
	if token == "" {
		return nil
	}

	if err := store.AccessToken(s.ctx).
		Where("token_hash = ?", HashToken(token)).
		Delete(); err != nil {
		return s.ctx.NewError(err, err)
	}

	// Whose session ended comes from the token the guard already resolved, not
	// from the token itself. An unknown token reaches here too and logs the same
	// line with no user — which is honest: the caller asked to be signed out and
	// they are.
	s.ctx.Log().Info("signed out", "user_id", currentUserID(s.ctx))

	return nil
}

// currentUserID is who the context says is calling, or "" for an anonymous or
// unresolved caller. Only used to label a log line, so a missing user is not an
// error here.
func currentUserID(ctx core.IContext) string {
	if user := ctx.GetUser(); user != nil {
		return user.ID
	}

	return ""
}

// Me returns the signed-in user, read fresh rather than from whatever the token
// carried: a token outlives changes to the account it names, so cached claims
// would serve a stale name — or one belonging to a user since deleted.
func (s authService) Me() (*models.User, core.IError) {
	current := s.ctx.GetUser()
	if current == nil || current.ID == "" {
		return nil, errmsgs.Unauthorized
	}

	user, err := s.users.Find(current.ID)
	if err != nil {
		if errmsgs.IsNotFoundError(err) {
			return nil, errmsgs.Unauthorized // well-formed token, account gone
		}
		return nil, s.ctx.NewError(err, err)
	}

	return user, nil
}

// HashToken is the one place a token becomes a digest, so what Login stores and
// what the guard looks up can never drift apart.
//
// Plain SHA-256 is right here and bcrypt is not: a token is 256 bits of
// randomness, so there is nothing to brute-force, and this runs on every
// request — a deliberately slow hash would be a denial of service on ourselves.
// Passwords are the opposite case and use utils.HashPassword, which is bcrypt.
func HashToken(token string) string {
	return utils.SHA256(token)
}

// newToken returns a fresh random token. crypto/rand, not math/rand: a guessable
// token is the whole authentication system.
func newToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return utils.HexEncode(b), nil
}

// ResolveToken turns a token's SHA-256 digest into the user it belongs to. It is
// the lookup behind the guard, and runs on every authenticated request.
//
// Exported because it is the whole of "who is this caller" in one function: a
// second transport — a websocket upgrade, a gRPC interceptor — authenticates by
// calling this rather than by reimplementing it.
//
// The digest arrives already hashed: core.HashedTokenVerifier hashes whatever
// the request carried before calling this, so the plaintext token never reaches
// here and a leaked database yields nothing that resolves.
//
// It takes an App rather than a context because the guard runs before any
// handler: the request's context arrives per call and is bound here.
func ResolveToken(app *core.App, usersFor UsersFor) func(context.Context, string) (*core.ContextUser, error) {
	return func(reqCtx context.Context, tokenHash string) (*core.ContextUser, error) {
		ctx := app.NewContext(reqCtx, core.ModeHTTP)

		record, err := store.AccessToken(ctx).Where("token_hash = ?", tokenHash).FindOne()
		if err != nil {
			// Debug for all three rejections below: this runs on every
			// authenticated request, so an Info here would be a second request
			// log. The request line already records the 401; these say which of
			// the three reasons it was, for the ten minutes someone is asking.
			ctx.Log().Debug("token rejected", "reason", "unknown_or_revoked")

			return nil, errmsgs.Unauthorized // unknown token, or already revoked
		}
		// A distinct code, because the caller can act on it: the token was real,
		// it has simply run out, and the answer is to sign in again rather than
		// to go looking for a bug. Core's middleware passes an IError through
		// untouched, so the code survives to the client.
		if record.ExpiresAt == nil || record.ExpiresAt.Before(time.Now().UTC()) {
			ctx.Log().Debug("token rejected", "reason", "expired", "user_id", record.UserID)

			return nil, emsgs.TokenExpired
		}

		user, findErr := usersFor(ctx).Find(record.UserID)
		if findErr != nil {
			// A live token naming an account that is gone. Warn rather than
			// Debug: the token was valid, so this is a row that outlived its
			// owner, and a steady trickle of them means a delete path is not
			// revoking sessions.
			ctx.Log().Warn("token rejected", "reason", "account_missing", "user_id", record.UserID)

			return nil, errmsgs.Unauthorized // account deleted since it was issued
		}

		return &core.ContextUser{
			ID:    user.ID,
			Email: user.Email,
			Name:  user.FullName,
		}, nil
	}
}
