package user

import (
	"github.com/pskclub/mine-core-template/middlewares"
	"github.com/pskclub/mine-core-template/modules/user/handler"
	core "github.com/pskclub/mine-core/v2"
)

// Routes registers the user routes behind authentication.
//
// The guard is not imported from the auth module: doing that would close a
// cycle, since auth reads accounts through this module's service. It arrives
// through the middlewares package, which cmd.Modules wires before any route is
// registered — the one place that knows both modules.
//
// middlewares.AuthRequire(e) is repeated on every line on purpose. A group would apply it once
// and silently cover routes added later, which is safer — but it also means the
// only way to know whether a route is protected is to scroll up. Written out,
// each line answers that question by itself, and a route added without it is
// visible in review as a line that differs from its neighbours.
func (*Module) Routes(e *core.Server) {
	c := &handler.UserHandler{}

	e.GET("/users", c.Pagination, middlewares.AuthRequire(e))
	e.GET("/users/:id", c.Find, middlewares.AuthRequire(e))
	e.POST("/users", c.Create, middlewares.AuthRequire(e))
	e.POST("/users/bulk", c.CreateBulk, middlewares.AuthRequire(e))
	e.PUT("/users/:id", c.Update, middlewares.AuthRequire(e))
	e.DELETE("/users/:id", c.Delete, middlewares.AuthRequire(e))
}
