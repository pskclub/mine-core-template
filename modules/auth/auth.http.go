package auth

import (
	"github.com/pskclub/mine-core-template/middlewares"
	"github.com/pskclub/mine-core-template/modules/auth/handler"
	core "github.com/pskclub/mine-core/v2"
)

// Routes registers the sign-in endpoints.
//
// register and login are public by necessity — they are how a caller obtains a
// token at all. Everything after that carries middlewares.AuthRequire(e).
//
// Paths are written out in full rather than assembled from a group prefix: the
// point is that "/auth/logout" appears literally in the source, so anyone who
// sees that path in a log or a bug report can grep for it and land on this line.
func (m *Module) Routes(e *core.Server) {
	c := handler.NewAuthHandler(m.usersFor)

	e.POST("/auth/register", c.Register)
	e.POST("/auth/login", c.Login)
	e.POST("/auth/logout", c.Logout, middlewares.AuthRequire(e))
	e.GET("/auth/me", c.Me, middlewares.AuthRequire(e))
}
