package exchange_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pskclub/mine-core-template/testkit"
	"github.com/pskclub/mine-core/v2/coretest"
)

// serve starts the whole service — every module, assembled by the same cmd.Modules
// that main uses — with the rate provider pointed at a fake one.
func serve(t *testing.T, provider http.HandlerFunc) *coretest.Client {
	t.Helper()

	upstream := httptest.NewServer(provider)
	t.Cleanup(upstream.Close)

	anon, _ := testkit.Serve(t, coretest.WithEnv(map[string]string{
		"exchange_base_url": upstream.URL,
	}))

	return testkit.SignIn(t, anon)
}

func rates(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"base":"THB","date":"2026-07-29","rates":{"USD":0.0275}}`))
}

func TestExchangeAPI_requiresAuthentication(t *testing.T) {
	c := serve(t, rates)
	anon := coretest.NewClient(t, c.BaseURL())

	anon.Get("/exchange-rates/THB?quote=USD").RequireStatus(http.StatusUnauthorized)
}

func TestExchangeAPI_find(t *testing.T) {
	c := serve(t, rates)

	body := c.Get("/exchange-rates/thb?quote=usd").RequireStatus(http.StatusOK).Map()

	// lower case in, upper case out: the handler normalises before the service
	// sees it, so "usd" and "USD" are one cache entry rather than two
	assert.Equal(t, "THB", body["base"])
	assert.Equal(t, "USD", body["quote"])
	assert.Equal(t, 0.0275, body["rate"])
}

func TestExchangeAPI_rejectsAMalformedPair(t *testing.T) {
	c := serve(t, rates)

	// four letters is not a currency code, and that is answerable without
	// calling anybody
	res := c.Get("/exchange-rates/THBX?quote=USD").RequireStatus(http.StatusBadRequest).Map()
	assert.Equal(t, "INVALID_PARAMS", res["code"])

	// a missing quote is the same kind of mistake
	c.Get("/exchange-rates/THB").RequireStatus(http.StatusBadRequest)
}

// The provider's failure reaches the client as ours: a 503 they can retry, with
// no hint of whose API fell over.
func TestExchangeAPI_providerDown(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	res := c.Get("/exchange-rates/THB?quote=USD").
		RequireStatus(http.StatusServiceUnavailable).Map()

	assert.Equal(t, "EXCHANGE_UNAVAILABLE", res["code"])
}
