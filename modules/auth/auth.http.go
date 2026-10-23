package auth

import (
	"time"

	"github.com/pskclub/mine-core-template/middlewares"
	"github.com/pskclub/mine-core-template/modules/auth/handler"
	core "github.com/pskclub/mine-core/v2"
)

// Credential attempts per client address per minute. Ten is generous for a
// person and tight for a script: a password list cannot be worked through at
// ten tries a minute, and a user who mistypes twice never notices the cap.
//
// It counts in the cache, so it is shared by every api replica — and it fails
// open: a service with no CACHE_* configured, or a redis that is down, lets
// every attempt through rather than locking everyone out. The sign-in log
// lines (Warn, by rate) are the alert for that case.
const (
	credentialAttempts = 10
	credentialWindow   = time.Minute
)

// Routes registers the sign-in endpoints.
//
// register and login are public by necessity — they are how a caller obtains a
// token at all, and the reason they carry a rate limit instead: the one thing an
// anonymous caller can do here is guess, and the limit is what bounds guessing.
// Everything after that carries middlewares.AuthRequire(e).
//
// Paths are written out in full rather than assembled from a group prefix: the
// point is that "/auth/logout" appears literally in the source, so anyone who
// sees that path in a log or a bug report can grep for it and land on this line.
func (m *Module) Routes(e *core.Server) {
	c := handler.NewAuthHandler(m.usersFor)
	limited := core.RateLimit(e.App().Cache(), credentialAttempts, credentialWindow)

	e.POST("/auth/register", c.Register, limited)
	e.POST("/auth/login", c.Login, limited)
	e.POST("/auth/logout", c.Logout, middlewares.AuthRequire(e))
	e.GET("/auth/me", c.Me, middlewares.AuthRequire(e))
}
