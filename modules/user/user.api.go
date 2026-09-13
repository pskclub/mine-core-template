package user

import (
	"github.com/pskclub/mine-core-template/modules/user/service"
	core "github.com/pskclub/mine-core/v2"
)

// This file is the module's public surface: everything another package may
// reach, gathered in one place and forwarding to the implementation.
//
// The rule it exists to keep true is that a module is entered only through its
// top-level package. cmd.NewAPI imports modules/user and never
// modules/user/service, so the import graph stays one arrow per module rather
// than one per layer — and moving a type between handler/, service/ and store/
// stays an edit inside this directory. arch fails the build on an
// import that goes around it.
//
// Whatever is not listed here, no other module can call. Adding a line is
// therefore a deliberate widening of what this module promises, and shows up in
// review as one.

// IUserService is how every other module reads and writes accounts.
type IUserService = service.IUserService

// NewUserService opens the service on a context. The auth module receives this
// as its own auth.Users interface — the method set matches, so nothing has to be
// adapted and neither module imports the other.
func NewUserService(ctx core.IContext) IUserService {
	return service.NewUserService(ctx)
}
