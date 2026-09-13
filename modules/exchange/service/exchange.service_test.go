package service_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pskclub/mine-core-template/modules/exchange/service"
	"github.com/pskclub/mine-core-template/testkit"
	"github.com/pskclub/mine-core/v2/coretest"
)

// The provider is faked with an httptest.Server rather than with a mocked
// IRequester: both ends are real, only the far end is ours. That is what makes
// these tests cover the query parameters we send, the JSON we decode, and the
// status codes we map — which is where this kind of code actually breaks.
func upstream(t *testing.T, h http.HandlerFunc) (string, *int32) {
	t.Helper()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		h(w, r)
	}))
	t.Cleanup(srv.Close)

	return srv.URL, &calls
}

// serviceAgainst opens the service on a context pointed at a fake provider.
// coretest.WithEnv sets the key for this test only, so nothing depends on the
// .env in anybody's checkout.
func serviceAgainst(t *testing.T, baseURL string) service.IExchangeService {
	t.Helper()

	ctx := testkit.Context(t, coretest.WithEnv(map[string]string{
		"exchange_base_url": baseURL,
	}))

	return service.NewExchangeService(ctx)
}

func okRates(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"base":"THB","date":"2026-07-29","rates":{"USD":0.0275}}`))
}

func TestExchangeService_Rate(t *testing.T) {
	var gotBase, gotSymbols string
	url, calls := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		gotBase = r.URL.Query().Get("base")
		gotSymbols = r.URL.Query().Get("symbols")
		okRates(w, r)
	})

	rate, err := serviceAgainst(t, url).Rate("THB", "USD")

	require.Nil(t, err)
	assert.Equal(t, "THB", gotBase, "the pair reaches the provider as query parameters")
	assert.Equal(t, "USD", gotSymbols)
	assert.Equal(t, 0.0275, rate.Rate)
	assert.Equal(t, "2026-07-29", rate.AsOf)
	assert.Equal(t, int32(1), atomic.LoadInt32(calls))
}

// A provider answers about a table of currencies, not about one pair. Reading
// the wrong entry out of it is a mistake nothing reports: the call succeeded,
// the status was 200, and the caller is simply quoted somebody else's currency.
func TestExchangeService_picksTheQuoteOutOfTheRateTable(t *testing.T) {
	url, calls := upstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// `success` is a field we do not model — a provider is free to send
		// more than we read, and doing so must not break the decode.
		_, _ = w.Write([]byte(`{"success":true,"base":"THB","date":"2026-07-29",
			"rates":{"USD":0.0275,"EUR":0.0253,"JPY":4.3121}}`))
	})

	rate, err := serviceAgainst(t, url).Rate("THB", "EUR")

	require.Nil(t, err)
	assert.Equal(t, 0.0253, rate.Rate, "the quote that was asked for, not the first key in the map")
	assert.Equal(t, "EUR", rate.Quote)
	assert.Equal(t, int32(1), atomic.LoadInt32(calls))
}

// What comes back is our DTO filled from what the caller asked for, not the
// provider's body renamed. Their echo of the pair is theirs to spell however
// they like — here in lower case — and a client of ours never sees it.
func TestExchangeService_answersAboutThePairTheCallerAsked(t *testing.T) {
	url, _ := upstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"base":"thb","date":"2026-07-29","rates":{"USD":0.0275}}`))
	})

	rate, err := serviceAgainst(t, url).Rate("THB", "USD")

	require.Nil(t, err)
	assert.Equal(t, "THB", rate.Base)
	assert.Equal(t, "USD", rate.Quote)
	assert.Equal(t, 0.0275, rate.Rate)
	assert.Equal(t, "2026-07-29", rate.AsOf, "the date is theirs: it is when the quote was struck")
}

// Where the call actually lands. The base URL is configuration and the path is
// not, so a provider configured with a version prefix is still called at
// <prefix>/latest — the one part of the URL a deployment cannot fix.
func TestExchangeService_callsLatestUnderTheConfiguredBaseURL(t *testing.T) {
	var gotMethod, gotPath string
	url, _ := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		okRates(w, r)
	})

	_, err := serviceAgainst(t, url+"/v1").Rate("THB", "USD")

	require.Nil(t, err)
	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Equal(t, "/v1/latest", gotPath)
}

// Both ends of the range a rate can take. A quote above one and a quote with
// eight decimals are where a decode quietly rounds, and a rate that is nearly
// right is worse than one that is missing.
func TestExchangeService_keepsTheRateItWasGiven(t *testing.T) {
	cases := []struct {
		name  string
		quote string
		body  string
		want  float64
	}{
		{"above one", "JPY", `{"base":"THB","date":"2026-07-29","rates":{"JPY":4.3121}}`, 4.3121},
		{"very small", "BTC", `{"base":"THB","date":"2026-07-29","rates":{"BTC":0.00000023}}`, 0.00000023},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url, _ := upstream(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})

			rate, err := serviceAgainst(t, url).Rate("THB", tc.quote)

			require.Nil(t, err)
			assert.Equal(t, tc.want, rate.Rate)
		})
	}
}

// Without redis configured there is no cache, and the service must still work —
// otherwise nobody can run it locally. `make test` is exactly that environment,
// which is why this asserts the provider is called twice here.
func TestExchangeService_worksWithoutACache(t *testing.T) {
	url, calls := upstream(t, okRates)
	svc := serviceAgainst(t, url)

	_, err := svc.Rate("THB", "USD")
	require.Nil(t, err)
	_, err = svc.Rate("THB", "USD")
	require.Nil(t, err)

	assert.Equal(t, int32(2), atomic.LoadInt32(calls))
}

// Their 404 is our 400: the request was well-formed, the pair simply is not one
// they quote — and the client can act on that.
func TestExchangeService_unknownCurrencyIsTheCallersMistake(t *testing.T) {
	url, _ := upstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"unknown_currency","message":"no such symbol"}}`))
	})

	_, err := serviceAgainst(t, url).Rate("THB", "XXX")

	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadRequest, err.GetStatus())
	assert.Equal(t, "UNKNOWN_CURRENCY", err.GetCode(),
		"the provider's code never reaches a client of ours")
}

// A 200 that does not contain the pair. Providers do this, and reading the zero
// value out of the map without checking is how a missing quote becomes a rate of
// 0.00 three layers away.
func TestExchangeService_missingQuoteInA200(t *testing.T) {
	url, _ := upstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"base":"THB","date":"2026-07-29","rates":{}}`))
	})

	_, err := serviceAgainst(t, url).Rate("THB", "USD")

	require.NotNil(t, err)
	assert.Equal(t, "UNKNOWN_CURRENCY", err.GetCode())
}

// A 200 whose body is not a rates response at all. A provider that starts
// demanding an API key answers exactly like this, and reporting it as
// UNKNOWN_CURRENCY tells the caller to fix a request that was never wrong —
// while the deployment mistake goes unnoticed because nothing logged an error.
func TestExchangeService_a200ThatIsNotARatesResponse(t *testing.T) {
	url, _ := upstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"error":{"code":101,"type":"missing_access_key"}}`))
	})

	_, err := serviceAgainst(t, url).Rate("THB", "USD")

	require.NotNil(t, err)
	assert.Equal(t, "EXCHANGE_UNAVAILABLE", err.GetCode(),
		"a provider that answers without rates is our problem, not the caller's")
}

// Throttling, their 500 and a refused connection are one answer to a client:
// come back later. The difference between them is ours to read in the log.
func TestExchangeService_providerFailuresBecomeOneError(t *testing.T) {
	cases := []struct {
		name string
		body string
		code int
	}{
		{"throttled", `{"retryable":true,"retry_after":30}`, http.StatusTooManyRequests},
		{"provider is broken", `{"error":{"code":"internal"}}`, http.StatusInternalServerError},
		{"gateway", `{}`, http.StatusBadGateway},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url, _ := upstream(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			})

			_, err := serviceAgainst(t, url).Rate("THB", "USD")

			require.NotNil(t, err)
			assert.Equal(t, http.StatusServiceUnavailable, err.GetStatus())
			assert.Equal(t, "EXCHANGE_UNAVAILABLE", err.GetCode())
		})
	}
}

// A deployment that was never finished says so, instead of answering 503 as if
// the provider were having a bad day.
func TestExchangeService_withoutAConfiguredProvider(t *testing.T) {
	_, err := serviceAgainst(t, "").Rate("THB", "USD")

	require.NotNil(t, err)
	assert.Equal(t, http.StatusInternalServerError, err.GetStatus())
	assert.Equal(t, "EXCHANGE_NOT_CONFIGURED", err.GetCode())
}
