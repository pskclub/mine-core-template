package handler

import (
	"regexp"
	"strings"

	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/valid"
)

// currencyCode is ISO 4217: three letters, always.
var currencyCode = regexp.MustCompile(`^[A-Za-z]{3}$`)

// FindRequest is `GET /exchange-rates/:base?quote=USD` — the path names what is
// being converted from, the query what to.
type FindRequest struct {
	Base  *string `param:"base" json:"-"`
	Quote *string `query:"quote" json:"-"`
}

// Valid checks the shape of the request and nothing about the world.
//
// Whether THB is a currency the upstream knows is not answerable here: it
// depends on a service that is not us, can change between this check and the
// call, and answering it would mean making the call twice. Three letters is
// what a currency code *looks* like, and that is the whole job — a well-formed
// code the upstream does not know comes back from the service as
// UNKNOWN_CURRENCY.
func (r *FindRequest) Valid(ctx core.IContext) core.IError {
	v := valid.New(ctx)

	v.Str("base", r.Base).Required().Match(currencyCode)
	v.Str("quote", r.Quote).Required().Match(currencyCode)

	return v.Error()
}

// Normalized returns the codes as the upstream wants them: upper case, trimmed.
//
// Done here rather than in the service so the service takes exactly what it
// needs, and so "usd" and "USD" are one cache key rather than two.
func (r *FindRequest) Normalized() (base, quote string) {
	return strings.ToUpper(strings.TrimSpace(*r.Base)),
		strings.ToUpper(strings.TrimSpace(*r.Quote))
}
