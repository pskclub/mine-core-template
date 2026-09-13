// Package auth issues and verifies the tokens the rest of the service trusts.
//
// It owns the access_tokens table and nothing else. Accounts live in the user
// module, and auth reaches them only through the narrow interface in
// service/auth.deps.go — see that file for why it is an interface and not an
// import.
//
// This directory holds no logic: what the module attaches to the service, one
// file per kind (auth.http.go, auth.jobs.go), plus auth.api.go — everything
// outside the module may call, including the token lookup that cmd.Modules
// installs as the authentication middleware. The work is in handler/, service/
// and store/, which nothing outside modules/auth imports.
package auth

// Module is everything auth attaches to the service: its routes and its
// scheduled sweep.
//
// It is the module that shows why core.IModule matters. The routes and the cron
// used to be registered in two different composition roots — cmd.NewAPI and
// cmd.newScheduler — so auth could be half-wired, and the half that was missing
// was the silent one: a token sweep nobody armed does not log anything, it just
// never runs, and access_tokens grows until somebody notices. Now both hang off
// this type, and the only way to attach one is to attach the module.
//
// They are still separate files: auth.http.go answers "which URLs does this
// serve" and auth.jobs.go answers "what runs on a clock", and each answer is
// worth a file whose name states it. What the split cannot do is lose one —
// neither method exists anywhere but this directory.
//
// Its dependency arrives through the constructor, exactly as it did as a
// function parameter before. A module declares what it needs as an interface of
// its own (service/auth.deps.go) and the composition root supplies it — which is
// also why there is no auto-registration: a module registered by an init() could
// never be given anything.
type Module struct {
	usersFor UsersFor
}

func New(usersFor UsersFor) *Module { return &Module{usersFor: usersFor} }

func (*Module) Name() string { return "auth" }
