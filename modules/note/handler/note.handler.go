// Package handler is the note module's HTTP edge: what arrives, how it is
// validated, and what is written back.
//
// It knows about requests and status codes; the service it calls knows about
// neither. Nothing outside modules/note imports this.
package handler

import (
	"net/http"

	"github.com/pskclub/mine-core-template/modules/note/service"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/errmsgs"
	"github.com/pskclub/mine-core/v2/utils"
)

// NoteHandler holds the module's HTTP handlers.
//
// A handler does four things and nothing else: bind, convert, call, render.
// Any line here that decides something belongs in the service, where a job can
// reach it too. Handlers return core.IError directly and the server renders it
// as {code, message, fields} with the error's own HTTP status.
type NoteHandler struct {
}

func (m NoteHandler) Pagination(c core.IHTTPContext) error {
	ownerID, err := callerID(c)
	if err != nil {
		return err
	}

	// The allowlist is what makes order_by safe — a column outside it is dropped
	// instead of reaching SQL.
	opts := c.GetPageOptionsWithAllowed("created_at", "title", "pinned")

	res, sErr := service.NewNoteService(c).Pagination(ownerID, opts)
	if sErr != nil {
		return sErr
	}

	return c.JSON(http.StatusOK, res)
}

func (m NoteHandler) Find(c core.IHTTPContext) error {
	ownerID, err := callerID(c)
	if err != nil {
		return err
	}

	note, sErr := service.NewNoteService(c).Find(ownerID, c.Param("id"))
	if sErr != nil {
		return sErr
	}

	return c.JSON(http.StatusOK, note)
}

func (m NoteHandler) Create(c core.IHTTPContext) error {
	ownerID, err := callerID(c)
	if err != nil {
		return err
	}

	input := &CreateRequest{}
	if bErr := c.BindWithValidate(input); bErr != nil {
		return bErr
	}

	payload, _ := utils.Copy[service.CreatePayload](input)

	note, sErr := service.NewNoteService(c).Create(ownerID, &payload)
	if sErr != nil {
		return sErr
	}

	return c.JSON(http.StatusCreated, note)
}

func (m NoteHandler) Update(c core.IHTTPContext) error {
	ownerID, err := callerID(c)
	if err != nil {
		return err
	}

	input := &UpdateRequest{}
	if bErr := c.BindWithValidate(input); bErr != nil {
		return bErr
	}

	payload, _ := utils.Copy[service.UpdatePayload](input)

	note, sErr := service.NewNoteService(c).Update(ownerID, *input.ID, &payload)
	if sErr != nil {
		return sErr
	}

	return c.JSON(http.StatusOK, note)
}

func (m NoteHandler) Delete(c core.IHTTPContext) error {
	ownerID, err := callerID(c)
	if err != nil {
		return err
	}

	if sErr := service.NewNoteService(c).Delete(ownerID, c.Param("id")); sErr != nil {
		return sErr
	}

	return c.NoContent(http.StatusNoContent)
}

// callerID is who the request is for. The middleware has already rejected a
// request without a valid token, so this cannot fail in practice — but reading a
// field off a pointer the compiler says may be nil deserves the check, and a
// route registered without middlewares.AuthRequire by mistake then answers 401
// rather than panicking.
//
// This is the only line a handler in this project writes. A handler binds,
// converts, calls and renders, and the framework already logs the method, path,
// status, latency and request id of every request — so "handling create note"
// would say nothing the request line does not.
//
// What earns a line is this: reaching here without a caller means a route was
// registered without its middleware, which the 401 alone would hide as an
// ordinary unauthenticated request.
func callerID(c core.IHTTPContext) (string, core.IError) {
	user := c.GetUser()
	if user == nil || user.ID == "" {
		c.Log().Error("route reached with no caller",
			"module", "note",
			"path", c.Request().URL.Path,
			"hint", "the route is missing middlewares.AuthRequire(e)")

		return "", errmsgs.Unauthorized
	}

	return user.ID, nil
}
