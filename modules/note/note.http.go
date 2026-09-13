package note

import (
	"github.com/pskclub/mine-core-template/middlewares"
	"github.com/pskclub/mine-core-template/modules/note/handler"
	core "github.com/pskclub/mine-core/v2"
)

// Routes registers every note route, and this file holds nothing else — so
// "which URLs does note serve" is a question answered by opening one file.
//
// Full paths, not a group prefix: "/notes/:id" is greppable, and a route's URL
// can be read without scrolling to find which group it was hung off. Every route
// carries middlewares.AuthRequire(e) explicitly for the same reason.
func (*Module) Routes(e *core.Server) {
	c := &handler.NoteHandler{}

	e.GET("/notes", c.Pagination, middlewares.AuthRequire(e))
	e.GET("/notes/:id", c.Find, middlewares.AuthRequire(e))
	e.POST("/notes", c.Create, middlewares.AuthRequire(e))
	e.PUT("/notes/:id", c.Update, middlewares.AuthRequire(e))
	e.DELETE("/notes/:id", c.Delete, middlewares.AuthRequire(e))
}
