package service

import "strings"

// Rate is what this module promises its callers.
//
// A type of ours, not the provider's shape passed through. The two are the same
// today by coincidence; the moment a second provider or a changed field arrives,
// every caller would have to change with it. This is the seam that keeps that
// change inside this package.
type Rate struct {
	Base  string  `json:"base"`
	Quote string  `json:"quote"`
	Rate  float64 `json:"rate"`
	AsOf  string  `json:"as_of"`
}

// ratesResponse is the provider's success body, and is not exported: nothing
// outside this package should be able to depend on their field names.
//
//	{"base":"THB","date":"2026-07-29","rates":{"USD":0.0275}}
type ratesResponse struct {
	Base  string             `json:"base"`
	Date  string             `json:"date"`
	Rates map[string]float64 `json:"rates"`
}

// upstreamError is the provider's failure body, bound with SetError.
//
// Send already turns a non-2xx into a core.IError carrying their status and
// code. This exists for what a status and a code cannot say: whether the failure
// is worth retrying, and how long to wait — the fields the service branches on.
//
//	{"error":{"code":"unknown_currency","message":"…","retry_after":30}}
type upstreamError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Retryable         bool `json:"retryable"`
	RetryAfterSeconds int  `json:"retry_after"`
}

// IsUnknownCurrency reads their code so the service does not have to know how it
// is spelled. Case-insensitive because it has been both.
func (e upstreamError) IsUnknownCurrency() bool {
	return strings.EqualFold(e.Error.Code, "unknown_currency")
}
