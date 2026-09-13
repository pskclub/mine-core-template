// Package service holds the exchange module's business rules — which here is
// almost entirely "how to call somebody else's API and survive it".
//
// Every call out of this process goes through core.Requester(ctx). What that
// buys, and what this file spends its length on:
//
//   - the caller's deadline. A request that has been abandoned stops the call it
//     was waiting on, instead of holding a connection for the upstream's timeout.
//   - the caller's trace. Every call leaves a breadcrumb on the request that
//     made it, and an issue in Sentry shows what we asked and what came back
//     without anyone logging it by hand.
//   - one client, one connection pool, configured once at startup.
//
// None of that survives a http.Get or a resty client built here per call.
package service

import (
	"context"
	"net/http"
	"time"

	"github.com/pskclub/mine-core-template/emsgs"
	core "github.com/pskclub/mine-core/v2"
)

const (
	// upstreamTimeout bounds one call to the rate provider. Shorter than the
	// client-wide default on purpose: a quote is worthless to the caller long
	// before 30 seconds, and a request that is going to fail should fail while
	// somebody is still waiting for it.
	upstreamTimeout = 5 * time.Second

	// rateTTL is how long a quote is reused. Rates move by the minute and this
	// endpoint is read far more often than that; the number is a trade between
	// freshness and the provider's quota, and it belongs here rather than in
	// config because it is a property of the data, not of the deployment.
	rateTTL = time.Minute

	// baseURLKey is APP_EXCHANGE_BASE_URL in the environment, or
	// exchange_base_url in .env — no prefix in the file, the prefix is for OS
	// variables. Ad hoc rather than a field on core.ENVConfig: it belongs to this
	// service, not to the framework.
	baseURLKey = "exchange_base_url"
)

// IExchangeService is this module's whole surface.
type IExchangeService interface {
	// Rate quotes base against quote — how much of quote one base buys.
	Rate(base, quote string) (*Rate, core.IError)
}

type exchangeService struct {
	ctx core.IContext
}

func NewExchangeService(ctx core.IContext) IExchangeService {
	return &exchangeService{ctx: ctx}
}

// Rate answers from the cache when it can, and calls the provider when it
// cannot.
//
// The cache is optional infrastructure: CACHE_CONNECTION_STRING may be unset, in
// which case ctx.Cache() is nil and this still works, only slower. A service
// that panics without redis is a service that cannot be run locally.
func (s exchangeService) Rate(base, quote string) (*Rate, core.IError) {
	cache := s.ctx.Cache()
	if cache == nil {
		return s.fetch(base, quote)
	}

	// core.Remember: return the cached value, or load and store it. A load
	// failure keeps its own status and code — core.Wrap preserves both — so the
	// mapping done in fetch survives the cache layer intact.
	return core.Remember(cache, cacheKey(base, quote), rateTTL, func() (*Rate, error) {
		return s.fetch(base, quote)
	})
}

// fetch is the call itself.
func (s exchangeService) fetch(base, quote string) (*Rate, core.IError) {
	baseURL := s.ctx.ENV().String(baseURLKey)
	if baseURL == "" {
		// A deployment mistake, not a client mistake: nobody outside can fix it
		// and nothing else will report it, so this is one of the few lines that
		// earns an Error.
		s.ctx.Log().Error("exchange rate provider is not configured",
			"key", baseURLKey,
			"hint", "set APP_EXCHANGE_BASE_URL, or EXCHANGE_BASE_URL in .env")

		return nil, emsgs.ExchangeNotConfigured
	}

	// A context of our own, so this one call is bounded without touching the
	// shared client's timeout — and core.Requester takes any context, which is
	// exactly why it is a function and not a method on the context.
	ctx, cancel := context.WithTimeout(s.ctx, upstreamTimeout)
	defer cancel()

	r := core.Requester(ctx)

	var out ratesResponse
	var fail upstreamError

	resp, err := r.Send(
		r.R().
			SetQueryParams(map[string]string{"base": base, "symbols": quote}).
			SetResult(&out). // body on success
			SetError(&fail), // body on failure — anything a code and a message cannot carry
		http.MethodGet, baseURL+"/latest",
	)
	if err != nil {
		return nil, s.upstreamFailed(err, &fail, base, quote)
	}

	if out.Rates == nil {
		// 200, and no `rates` key at all — so this is not a rates response. A
		// provider that has started demanding an API key answers exactly like
		// this, and the status code says nothing about it.
		//
		// The distinction from the branch below is the whole point: a body
		// without the key is theirs to fix, a body with the key but without the
		// pair is the caller's. Collapsing the two is how a missing credential
		// gets reported to a client as "that currency pair is not available".
		// Not fail.Error — SetError binds on non-2xx only, so on a 200 it is
		// still the zero value and logging it would print two empty fields.
		s.ctx.Log().Error("provider answered 200 without a rates object",
			"base", base, "quote", quote, "status", resp.StatusCode(),
			"hint", "the provider may now require credentials, or "+baseURLKey+" may point at the wrong path")

		return nil, emsgs.ExchangeUnavailable
	}

	// Rates present but not this pair: that *is* an answer, and the caller can
	// act on it. Reading the zero value out of the map without checking is how a
	// missing quote becomes a rate of 0.00 three layers away.
	rate, ok := out.Rates[quote]
	if !ok {
		s.ctx.Log().Warn("quote missing from a successful response",
			"base", base, "quote", quote, "status", resp.StatusCode())

		return nil, emsgs.UnknownCurrency.WithFields(map[string]any{"quote": quote})
	}

	return &Rate{Base: base, Quote: quote, Rate: rate, AsOf: out.Date}, nil
}

// upstreamFailed turns their failure into ours.
//
// The IError from Send already carries their status and, when the body gave one,
// their code — which is exactly what must not be handed to our clients: a caller
// branching on the provider's error codes is a caller that breaks when we change
// provider. So every outcome maps to an error this service defines, and the
// original is kept as the cause for the log and for Sentry.
func (s exchangeService) upstreamFailed(err core.IError, fail *upstreamError, base, quote string) core.IError {
	log := s.ctx.Log().With("base", base, "quote", quote,
		"upstream_status", err.GetStatus(), "upstream_code", err.GetCode())

	switch {
	case fail.IsUnknownCurrency() || err.GetStatus() == http.StatusNotFound:
		// Their 404 is our 400: the request was well-formed, the currency simply
		// is not one they quote. Nothing is wrong on our side, and the caller can
		// fix it by asking for a different pair.
		log.Warn("unknown currency pair")

		return emsgs.UnknownCurrency.WithFields(map[string]any{"base": base, "quote": quote})

	case err.GetStatus() == http.StatusTooManyRequests || fail.Retryable:
		// Ours, not theirs: we are over the quota we bought. 503 tells the caller
		// to come back, which is true, without exposing that we buy this data.
		log.Warn("rate provider is throttling us", "retry_after_seconds", fail.RetryAfterSeconds)

		return s.ctx.NewError(err, emsgs.ExchangeUnavailable)

	default:
		// Anything else — their 5xx, a timeout, a connection refused. NewError
		// reports it to Sentry with this request's user and trace, because a
		// dependency that is down is our problem to notice.
		return s.ctx.NewError(err, emsgs.ExchangeUnavailable)
	}
}

func cacheKey(base, quote string) string {
	return "exchange:rate:" + base + ":" + quote
}
