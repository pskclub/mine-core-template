// Package user owns accounts: the users table, and every read and write of it.
//
// Other modules never query that table. They call IUserService, which is the
// only door in — see store/user.repo.go for why the repository lives in a
// package nothing outside modules/user may import.
//
// This directory holds what the module attaches to the service, one file per
// kind (today only user.http.go), plus user.api.go — everything outside the
// module may call. The work is in handler/, service/ and store/.
package user

type Module struct{}

func New() *Module { return &Module{} }

func (*Module) Name() string { return "user" }
