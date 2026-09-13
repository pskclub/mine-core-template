// Package home answers the root route, and is the smallest a module gets: no
// service, no repository, no table.
//
// Not everything needs the full shape. A module has as many of the parts as it
// has work for — see modules/note for one that has all of them.
package home

import (
	"net/http"

	core "github.com/pskclub/mine-core/v2"
)

// Module is the whole of home, and the smallest a core.IModule gets: a name and
// a route. Every other capability — jobs, cron, consumers, health checks — is an
// optional interface, so a module implements exactly what it has.
//
// Routes stays in this file rather than a home.http.go of its own, which the
// other modules have. The rule is that no attachment point lives outside the
// module's directory, not that each gets a file; splitting is what keeps a file
// name honest once there is more than one kind, and here there is only ever one.
type Module struct{}

func New() *Module { return &Module{} }

func (*Module) Name() string { return "home" }

func (*Module) Routes(e *core.Server) {
	e.GET("/", Status)
}

// The health probes used to be registered here. They are in cmd.NewAPI now,
// because the readiness probe has to be handed every module's own checks
// (mods.HealthChecks()) and only the composition root holds them. Registering
// them from inside one module would give a probe reporting fewer dependencies
// than the service has — and saying nothing about being incomplete.

func Status(c core.IHTTPContext) error {
	return c.JSON(http.StatusOK, map[string]any{
		"status": "i'm ok na",
	})
}
