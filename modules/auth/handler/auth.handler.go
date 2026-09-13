// Package handler is the auth module's HTTP edge: what arrives, how it is
// validated, and what is written back.
//
// It knows about requests and status codes; the service it calls knows about
// neither. Nothing outside modules/auth imports this — the routes in
// modules/auth are the only caller.
package handler

import (
	"net/http"
	"strings"

	"github.com/pskclub/mine-core-template/modules/auth/service"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/utils"
)

// AuthHandler holds the module's dependencies, not per-request state. A
// method builds its service from the request context plus what is stored here.
type AuthHandler struct {
	usersFor service.UsersFor
}

func NewAuthHandler(usersFor service.UsersFor) *AuthHandler {
	return &AuthHandler{usersFor: usersFor}
}

func (m AuthHandler) service(c core.IHTTPContext) service.IAuthService {
	return service.NewAuthService(c, m.usersFor(c))
}

func (m AuthHandler) Register(c core.IHTTPContext) error {
	input := &RegisterRequest{}
	if err := c.BindWithValidate(input); err != nil {
		return err
	}

	payload, _ := utils.Copy[service.RegisterPayload](input)

	user, err := m.service(c).Register(&payload)
	if err != nil {
		return err
	}

	// no token here on purpose: registering and signing in stay separate, so
	// adding email confirmation later does not change what this returns
	return c.JSON(http.StatusCreated, user)
}

func (m AuthHandler) Login(c core.IHTTPContext) error {
	input := &LoginRequest{}
	if err := c.BindWithValidate(input); err != nil {
		return err
	}

	payload, _ := utils.Copy[service.LoginPayload](input)

	res, err := m.service(c).Login(&payload)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

// Logout revokes the token that made the request, so it stops working at once.
func (m AuthHandler) Logout(c core.IHTTPContext) error {
	if err := m.service(c).Logout(bearerToken(c)); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// Me answers for whoever the token names. The guard has already rejected a
// missing, unknown or expired one, so reaching here means it verified.
func (m AuthHandler) Me(c core.IHTTPContext) error {
	user, err := m.service(c).Me()
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, user)
}

// bearerToken reads the token back off the request. Logout needs the token
// itself, not the user it resolved to — the guard keeps only the latter.
func bearerToken(c core.IHTTPContext) string {
	const prefix = "Bearer "

	h := c.Request().Header.Get("Authorization")
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return h[len(prefix):]
	}
	return ""
}
