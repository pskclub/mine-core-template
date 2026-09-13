// Package note is the worked example of a module in this project.
//
// It is a real, working feature — notes belonging to the signed-in user — kept
// deliberately small so the shape is easy to read. See modules/note/README.md
// for what each directory is for and which rules it demonstrates.
//
// Copy this module when starting a new one, or run `make new-module name=order`,
// which generates the same layout with the names substituted.
//
// This directory holds only what the module attaches to the service, one file
// per kind — today that is note.http.go and nothing else. The work is in
// handler/, service/ and store/, which nothing outside modules/note imports —
// and this module offers nothing to other modules, so it has no note.api.go.
// Compare modules/user, which does.
package note

// Module is everything note attaches to the service. Today that is routes; if it
// grows a cron sweep or a queue consumer, the method for it goes in note.jobs.go
// or note.mq.go beside this file rather than in another composition root — which
// is the whole reason core.IModule exists. A feature registered in two places can
// be half-registered, and the missing half is silent.
//
// One file per kind rather than all of them here, so a file name stays true:
// note.http.go holds routes and only routes. The split is presentation; what it
// must not do is let an attachment point escape this directory, and that is the
// guarantee whether the module has one file or four.
type Module struct{}

func New() *Module { return &Module{} }

func (*Module) Name() string { return "note" }
