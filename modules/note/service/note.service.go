// Package service holds the note module's business rules.
//
// It is imported by this module's handler and by modules/note itself, and by
// nothing else.
package service

import (
	"github.com/pskclub/mine-core-template/emsgs"
	"github.com/pskclub/mine-core-template/models"
	"github.com/pskclub/mine-core-template/modules/note/store"
	"github.com/pskclub/mine-core-template/repo"
	core "github.com/pskclub/mine-core/v2"
)

// maxNotesPerUser is a business rule, enforced here rather than in the request:
// it depends on rows the payload cannot see.
const maxNotesPerUser = 200

// INoteService is this module's whole surface.
//
// Every method takes ownerID first. Threading it through explicitly rather than
// reading c.GetUser() inside the service is what lets a job or an admin tool
// call this code — and it puts the ownership rule in the signature, where it is
// impossible to call the method and forget it.
type INoteService interface {
	Create(ownerID string, input *CreatePayload) (*models.Note, core.IError)
	Update(ownerID, id string, input *UpdatePayload) (*models.Note, core.IError)
	Find(ownerID, id string) (*models.Note, core.IError)
	Pagination(ownerID string, pageOptions *core.PageOptions) (*core.Page[models.Note], core.IError)
	Delete(ownerID, id string) core.IError
}

type noteService struct {
	ctx core.IContext
}

// NewNoteService stores the context and nothing else. The logger is taken from
// it at the point of use — ctx.Log() already carries the request id, so there is
// nothing a stored copy would add.
func NewNoteService(ctx core.IContext) INoteService {
	return &noteService{ctx: ctx}
}

func (s noteService) Create(ownerID string, input *CreatePayload) (*models.Note, core.IError) {
	count, err := store.Note(s.ctx).Scopes(store.OwnedBy(ownerID)).Count()
	if err != nil {
		return nil, s.ctx.NewError(err, err)
	}
	if count >= maxNotesPerUser {
		// A rule refusing a well-formed request. Warn, not Error: the service is
		// working exactly as designed, but this is the line that turns "a user
		// says they cannot save anything" into an answer in one search.
		s.ctx.Log().Warn("note rejected", "reason", "limit_reached", "owner_id", ownerID, "count", count)

		return nil, emsgs.NoteLimitReached
	}

	note := &models.Note{
		BaseModel: models.NewBaseModel(),
		UserID:    ownerID,
		Title:     input.Title,
		Body:      input.Body,
		Pinned:    input.Pinned,
	}

	if err := store.Note(s.ctx).Create(note); err != nil {
		return nil, s.ctx.NewError(err, err)
	}

	// The ids, not the title or the body: what the user wrote is theirs, and a
	// log line is the wrong place for it.
	s.ctx.Log().Info("note created", "note_id", note.ID, "owner_id", ownerID)

	return s.Find(ownerID, note.ID)
}

func (s noteService) Update(ownerID, id string, input *UpdatePayload) (*models.Note, core.IError) {
	// Find first, with the ownership scope: someone else's id fails here, as a
	// 404, before any write is attempted.
	if _, findErr := s.Find(ownerID, id); findErr != nil {
		return nil, findErr
	}

	// A map, not the struct. Updates() with a struct skips zero values, so
	// unpinning a note — Pinned going from true to false — would be silently
	// dropped and the row would come back still pinned. Any column that can
	// legitimately hold a zero value (false, 0, "") has to be written this way.
	//
	// The ownership scope is applied again rather than trusted from the read
	// above: these are two separate statements, and a filter present on only one
	// of them is the shape most ownership bugs take.
	if err := store.Note(s.ctx).Scopes(store.OwnedBy(ownerID)).Where("id = ?", id).
		Updates(map[string]any{
			"title":  input.Title,
			"body":   input.Body,
			"pinned": input.Pinned,
		}); err != nil {
		return nil, s.ctx.NewError(err, err)
	}

	s.ctx.Log().Info("note updated", "note_id", id, "owner_id", ownerID)

	return s.Find(ownerID, id)
}

// Find answers 404 for a note belonging to someone else, exactly as it does for
// one that does not exist. A 403 would confirm the id is real, which is a fact
// the caller has no right to.
func (s noteService) Find(ownerID, id string) (*models.Note, core.IError) {
	note, err := store.Note(s.ctx).Scopes(store.OwnedBy(ownerID)).FindOne("id = ?", id)
	if err != nil {
		return nil, s.ctx.NewError(err, err)
	}

	return note, nil
}

func (s noteService) Pagination(ownerID string, pageOptions *core.PageOptions) (*core.Page[models.Note], core.IError) {
	opts := repo.DefaultOrder(pageOptions, "pinned DESC", "created_at DESC")

	page, err := store.Note(s.ctx).
		Scopes(store.OwnedBy(ownerID), store.Search(opts.Q)).
		Pagination(opts)
	if err != nil {
		return nil, s.ctx.NewError(err, err)
	}

	// Debug: one line per list request is not worth Info, but "whose list, and
	// filtered by what" is the first thing asked when a list looks wrong.
	s.ctx.Log().Debug("notes listed", "owner_id", ownerID, "total", page.Total, "q", opts.Q)

	return page, nil
}

func (s noteService) Delete(ownerID, id string) core.IError {
	if _, err := s.Find(ownerID, id); err != nil {
		return err
	}

	if err := store.Note(s.ctx).Scopes(store.OwnedBy(ownerID)).Delete("id = ?", id); err != nil {
		return s.ctx.NewError(err, err)
	}

	s.ctx.Log().Info("note deleted", "note_id", id, "owner_id", ownerID, "soft", true)

	return nil
}
