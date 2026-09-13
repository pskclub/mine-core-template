// Package service holds the user module's business rules.
//
// It is imported by this module's handler and by modules/user itself, and by
// nothing else — another module reaches accounts through modules/user.
package service

import (
	"github.com/pskclub/mine-core-template/models"
	"github.com/pskclub/mine-core-template/modules/user/store"
	"github.com/pskclub/mine-core-template/repo"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/repository"
	"gorm.io/gorm"
)

// createBatchSize bounds one INSERT. Every driver has a parameter limit, and a
// single statement for hundreds of rows hits it long before the row count does.
const createBatchSize = 50

// IUserService is this module's whole surface — to its own handler and to
// every other module. Nothing outside this package touches the users table
// except through one of these methods.
type IUserService interface {
	Create(input *CreatePayload) (*models.User, core.IError)
	CreateMany(inputs []*CreatePayload) ([]models.User, core.IError)
	Update(id string, input *UpdatePayload) (*models.User, core.IError)
	Find(id string) (*models.User, core.IError)
	Pagination(pageOptions *core.PageOptions) (*core.Page[models.User], core.IError)
	Delete(id string) core.IError

	// FindByEmail and CreateAccount exist for the auth module, which cannot
	// reach this table itself. They take and return nothing from this package,
	// so auth can describe them in its own interface without importing us —
	// which it must not do, since its guard is already on our routes.
	FindByEmail(email string) (*models.User, core.IError)
	CreateAccount(email, fullName, passwordHash string) (*models.User, core.IError)
}

type userService struct {
	ctx core.IContext
}

// NewUserService takes any core.IContext — the same service runs unchanged in an
// HTTP handler, a job or a test.
//
// Nothing else is stored. The logger comes off the context at the point of use:
// ctx.Log() is already bound to the unit of work — the request id under HTTP,
// the job name and run id under the scheduler — so a field holding a copy would
// carry the same thing and be one more member to thread through.
func NewUserService(ctx core.IContext) IUserService {
	return &userService{ctx: ctx}
}

func (s userService) Create(input *CreatePayload) (*models.User, core.IError) {
	user := &models.User{
		BaseModel: models.NewBaseModel(),
		Email:     input.Email,
		FullName:  input.FullName,
		Password:  input.PasswordHash,
	}

	// The failure is not logged here. It is returned, and the framework logs the
	// request with its status and code and reports the 5xx to Sentry — logging
	// it as well would put the same fault in the log twice, under two messages,
	// and make it look like two faults.
	//
	// NewError(err, err) keeps the repository's own status and code (404 stays a
	// 404) while reporting 5xx to Sentry here, where the context still knows the
	// user and the request.
	if err := store.User(s.ctx).Create(user); err != nil {
		return nil, s.ctx.NewError(err, err)
	}

	// A write that changed data is worth a line: the request log says a POST
	// returned 201, this says which row now exists. The id and not the email —
	// an address is personal data, and the id is what an investigation actually
	// joins on.
	s.ctx.Log().Info("user created", "user_id", user.ID)

	return s.Find(user.ID)
}

// CreateMany inserts every user or none of them.
//
// A batch that failed halfway would leave the caller with some users created and
// no way to know which, so a retry would duplicate them.
//
// GORM already wraps CreateInBatches in a transaction, so this is belt and
// braces — but the guarantee is the point of the method, and stating it here
// keeps it true if SkipDefaultTransaction is ever set, or if this method grows a
// second write. Inside the transaction the repository is rebound to tx;
// store.User(s.ctx) would take its own connection and write outside it.
func (s userService) CreateMany(inputs []*CreatePayload) ([]models.User, core.IError) {
	if len(inputs) == 0 {
		return []models.User{}, nil
	}

	users := make([]models.User, 0, len(inputs))
	for _, input := range inputs {
		users = append(users, models.User{
			BaseModel: models.NewBaseModel(),
			Email:     input.Email,
			FullName:  input.FullName,
			Password:  input.PasswordHash,
		})
	}

	err := store.User(s.ctx).Transaction(func(tx *gorm.DB) error {
		return repository.NewWithDB[models.User](s.ctx, tx).CreateInBatches(&users, createBatchSize)
	})
	if err != nil {
		return nil, s.ctx.NewError(err, err)
	}

	// The count, not the ids: a line is read by a human and shipped to a log
	// store that charges by the byte, and a hundred ids serve neither.
	s.ctx.Log().Info("users created in bulk", "count", len(users))

	return users, nil
}

func (s userService) Update(id string, input *UpdatePayload) (*models.User, core.IError) {
	user, err := s.Find(id)
	if err != nil {
		return nil, err
	}

	user.FullName = input.FullName // GORM refreshes UpdatedAt on its own

	if err := store.User(s.ctx).Where("id = ?", id).Updates(user); err != nil {
		return nil, s.ctx.NewError(err, err)
	}

	s.ctx.Log().Info("user updated", "user_id", id)

	return s.Find(user.ID)
}

// Find writes no line. Reads are most of the traffic, and the request log
// already records that a GET happened and what it answered — a line per read
// would triple the volume and say nothing new.
func (s userService) Find(id string) (*models.User, core.IError) {
	user, err := store.User(s.ctx).FindOne("id = ?", id)
	if err != nil {
		return nil, s.ctx.NewError(err, err)
	}

	return user, nil
}

// FindByEmail is the sign-in lookup. It returns the full row, password hash
// included — auth has nothing to compare a password against otherwise. That is
// also why it is not exposed over HTTP anywhere: `json:"-"` keeps the hash out
// of a response, but a method that hands it to Go code has to be deliberate.
func (s userService) FindByEmail(email string) (*models.User, core.IError) {
	user, err := store.User(s.ctx).Where("email = ?", email).FindOne()
	if err != nil {
		return nil, s.ctx.NewError(err, err)
	}

	return user, nil
}

// CreateAccount is registration, called by auth. It takes an already-hashed
// password: this module must never be in a position to store a plaintext one by
// mistake, so the plaintext never crosses the boundary at all.
//
// It adds no line of its own: Create writes "user created", and auth writes the
// security event. Two modules, two facts, joined by the request id.
func (s userService) CreateAccount(email, fullName, passwordHash string) (*models.User, core.IError) {
	return s.Create(&CreatePayload{
		Email:        email,
		FullName:     fullName,
		PasswordHash: passwordHash,
	})
}

func (s userService) Pagination(pageOptions *core.PageOptions) (*core.Page[models.User], core.IError) {
	opts := repo.DefaultOrder(pageOptions, "created_at DESC")

	page, err := store.User(s.ctx).
		Scopes(store.Search(opts.Q)).
		Pagination(opts)
	if err != nil {
		return nil, s.ctx.NewError(err, err)
	}

	// Debug, not Info: this runs on every list request, and it exists for one
	// question — "the list came back empty, what did we actually ask for". That
	// question is asked while investigating, which is when debug is turned on.
	s.ctx.Log().Debug("users listed", "total", page.Total, "q", opts.Q, "page", opts.Page, "limit", opts.Limit)
	return page, nil
}

func (s userService) Delete(id string) core.IError {
	if _, err := s.Find(id); err != nil {
		return err
	}

	if err := store.User(s.ctx).Delete("id = ?", id); err != nil {
		return s.ctx.NewError(err, err)
	}

	// Info and not Debug: a row disappearing from every read is the kind of
	// thing someone comes looking for months later. `soft` says it is still
	// recoverable, which is the first thing they will want to know.
	s.ctx.Log().Info("user deleted", "user_id", id, "soft", true)

	return nil
}
