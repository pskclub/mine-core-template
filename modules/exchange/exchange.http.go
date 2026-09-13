package exchange

import (
	"github.com/pskclub/mine-core-template/middlewares"
	"github.com/pskclub/mine-core-template/modules/exchange/handler"
	core "github.com/pskclub/mine-core/v2"
)

// Routes registers every exchange route.
func (*Module) Routes(e *core.Server) {
	c := &handler.ExchangeHandler{}

	e.GET("/exchange-rates/:base", c.Find, middlewares.AuthRequire(e))
}
