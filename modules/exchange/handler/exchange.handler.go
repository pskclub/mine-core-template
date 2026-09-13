// Package handler is the exchange module's HTTP edge.
//
// It knows about requests and status codes; the service it calls knows about
// neither. Nothing outside modules/exchange imports this.
package handler

import (
	"net/http"

	"github.com/pskclub/mine-core-template/modules/exchange/service"
	core "github.com/pskclub/mine-core/v2"
)

// ExchangeHandler holds the module's HTTP handlers.
type ExchangeHandler struct {
}

// Find quotes one currency against another.
//
//	GET /exchange-rates/THB?quote=USD
//
// Four lines of work — bind, convert, call, render — and no decision among them.
// Whether the answer comes from the cache or from the provider, and what a
// provider failure means to a client, are the service's to know.
func (m ExchangeHandler) Find(c core.IHTTPContext) error {
	input := &FindRequest{}
	if err := c.BindWithValidate(input); err != nil {
		return err
	}

	base, quote := input.Normalized()

	rate, sErr := service.NewExchangeService(c).Rate(base, quote)
	if sErr != nil {
		return sErr
	}

	return c.JSON(http.StatusOK, rate)
}
