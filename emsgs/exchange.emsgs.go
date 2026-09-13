package emsgs

import (
	"net/http"

	core "github.com/pskclub/mine-core/v2"
)

// Errors raised by the exchange module — every one of them a translation of
// somebody else's failure into a fact about *this* service.
//
// The rule these exist to keep: a client never sees the provider's error codes.
// A caller branching on them is a caller that breaks the day we change provider,
// and it leaks which provider we buy from.
var (
	// UnknownCurrency is a well-formed code the provider does not quote.
	//
	// 400, not 404: the pair is a parameter of the request, not a resource that
	// was looked for and missing. The client can fix it by asking for another.
	UnknownCurrency = core.New(
		http.StatusBadRequest,
		"UNKNOWN_CURRENCY",
		"that currency pair is not available",
	)

	// ExchangeUnavailable is every way the provider can fail to answer: down,
	// timing out, throttling us, or 500ing.
	//
	// One error for all of them because the client's move is the same in every
	// case — try again later — and the difference between them is ours to see in
	// the log, not theirs to act on.
	ExchangeUnavailable = core.New(
		http.StatusServiceUnavailable,
		"EXCHANGE_UNAVAILABLE",
		"exchange rates are temporarily unavailable",
	)

	// ExchangeNotConfigured means this deployment has no provider URL.
	//
	// A 500 and not a 503: nothing will get better by waiting, and calling it
	// temporary would hide a deployment that was never finished.
	ExchangeNotConfigured = core.New(
		http.StatusInternalServerError,
		"EXCHANGE_NOT_CONFIGURED",
		"exchange rates are not configured for this environment",
	)
)
