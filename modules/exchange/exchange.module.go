// Package exchange reads foreign-exchange rates from a third-party API.
//
// It is the worked example of a module whose data lives somewhere else: no
// table, no prisma schema, no store/ — the "repository" is an HTTP call, and
// everything that makes such a call survivable is in service/exchange.service.go.
//
// A module has as many of the parts as it has work for. modules/note has all of
// them, modules/home has one file, and this one sits in between: routes, a
// handler that binds and renders, and a service that owns the call.
package exchange

type Module struct{}

func New() *Module { return &Module{} }

func (*Module) Name() string { return "exchange" }
