// Package handler is the user module's HTTP edge: what arrives, how it is
// validated, and what is written back.
//
// It knows about requests and status codes; the service it calls knows about
// neither. Nothing outside modules/user imports this — the routes in
// modules/user are the only caller.
package handler

import (
	"net/http"

	"github.com/pskclub/mine-core-template/modules/user/service"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/utils"
)

// UserHandler holds the module's HTTP handlers.
//
// Handlers return core.IError directly: the server renders it as
// {code, message, fields} with the error's own HTTP status.
type UserHandler struct {
}

func (m UserHandler) Pagination(c core.IHTTPContext) error {
	// The allowlist is what makes order_by safe — a column outside it is dropped
	// instead of reaching SQL.
	opts := c.GetPageOptionsWithAllowed("created_at", "email", "full_name")

	res, err := service.NewUserService(c).Pagination(opts)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (m UserHandler) Find(c core.IHTTPContext) error {
	user, err := service.NewUserService(c).Find(c.Param("id"))
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, user)
}

func (m UserHandler) Create(c core.IHTTPContext) error {
	input := &CreateRequest{}
	if err := c.BindWithValidate(input); err != nil {
		return err
	}

	payload, _ := utils.Copy[service.CreatePayload](input)

	user, err := service.NewUserService(c).Create(&payload)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, user)
}

// CreateBulk creates many users in one call. Validation reports per-entry
// violations as users.<index>.<field>, so the caller knows which entry to fix,
// and the service inserts all of them or none.
func (m UserHandler) CreateBulk(c core.IHTTPContext) error {
	input := &CreateBulkRequest{}
	if err := c.BindWithValidate(input); err != nil {
		return err
	}

	payloads := make([]*service.CreatePayload, 0, len(input.Users))
	for _, u := range input.Users {
		payload, _ := utils.Copy[service.CreatePayload](u)
		payloads = append(payloads, &payload)
	}

	users, err := service.NewUserService(c).CreateMany(payloads)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"items": users,
		"count": len(users),
	})
}

func (m UserHandler) Update(c core.IHTTPContext) error {
	// ID is bound from the path (`param:"id"`) and validated with the body.
	input := &UpdateRequest{}
	if err := c.BindWithValidate(input); err != nil {
		return err
	}

	payload, _ := utils.Copy[service.UpdatePayload](input)

	user, err := service.NewUserService(c).Update(*input.ID, &payload)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, user)
}

func (m UserHandler) Delete(c core.IHTTPContext) error {
	if err := service.NewUserService(c).Delete(c.Param("id")); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
