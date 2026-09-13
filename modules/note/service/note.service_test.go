package service_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pskclub/mine-core-template/models"
	"github.com/pskclub/mine-core-template/modules/note/service"
	"github.com/pskclub/mine-core-template/testkit"
	core "github.com/pskclub/mine-core/v2"
)

// A service test needs no HTTP and no token: ownership is a parameter, so the
// rule can be exercised directly. That is the payoff for threading ownerID
// through the signatures instead of reading it off the request inside.
func owners(t *testing.T, ctx core.IContext) (string, string) {
	t.Helper()

	seeded := testkit.SeedUsers(t, ctx, "owner@example.com", "stranger@example.com")

	return seeded[0].ID, seeded[1].ID
}

func TestNoteService_Create(t *testing.T) {
	ctx := testkit.Context(t)
	owner, _ := owners(t, ctx)

	n, err := service.NewNoteService(ctx).Create(owner, &service.CreatePayload{
		Title:  "First",
		Body:   "hello",
		Pinned: true,
	})

	require.Nil(t, err)
	assert.NotEmpty(t, n.ID, "the service assigns the id")
	assert.Equal(t, owner, n.UserID, "the owner comes from the caller, never from the payload")
	assert.True(t, n.Pinned)
}

func TestNoteService_ownershipIsEnforcedOnEveryOperation(t *testing.T) {
	ctx := testkit.Context(t)
	owner, stranger := owners(t, ctx)
	svc := service.NewNoteService(ctx)

	n, err := svc.Create(owner, &service.CreatePayload{Title: "Private", Body: "x"})
	require.Nil(t, err)

	t.Run("find", func(t *testing.T) {
		_, fErr := svc.Find(stranger, n.ID)

		require.NotNil(t, fErr)
		assert.Equal(t, http.StatusNotFound, fErr.GetStatus(),
			"someone else's note is missing, not forbidden")
	})

	t.Run("update", func(t *testing.T) {
		_, uErr := svc.Update(stranger, n.ID, &service.UpdatePayload{Title: "Hijacked", Body: "x"})

		require.NotNil(t, uErr)
		assert.Equal(t, http.StatusNotFound, uErr.GetStatus())
	})

	t.Run("delete", func(t *testing.T) {
		dErr := svc.Delete(stranger, n.ID)

		require.NotNil(t, dErr)
		assert.Equal(t, http.StatusNotFound, dErr.GetStatus())
	})

	t.Run("list", func(t *testing.T) {
		page, pErr := svc.Pagination(stranger, &core.PageOptions{Limit: 10, Page: 1})

		require.Nil(t, pErr)
		assert.Equal(t, int64(0), page.Total)
	})

	// after all of that, the note is exactly as it was
	unchanged, fErr := svc.Find(owner, n.ID)
	require.Nil(t, fErr)
	assert.Equal(t, "Private", unchanged.Title)
}

func TestNoteService_Delete_isSoft(t *testing.T) {
	ctx := testkit.Context(t)
	owner, _ := owners(t, ctx)
	svc := service.NewNoteService(ctx)

	n, err := svc.Create(owner, &service.CreatePayload{Title: "Gone", Body: "x"})
	require.Nil(t, err)

	require.Nil(t, svc.Delete(owner, n.ID))

	_, fErr := svc.Find(owner, n.ID)
	require.NotNil(t, fErr)
	assert.Equal(t, http.StatusNotFound, fErr.GetStatus())

	var count int64
	require.NoError(t, ctx.DB().Unscoped().Model(&models.Note{}).
		Where("id = ?", n.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "delete is soft: the row survives with deleted_at set")
}

// The ceiling is a business rule, so it is enforced in the service where it can
// see the rows — and it is counted per owner, not globally.
func TestNoteService_Create_perOwnerLimit(t *testing.T) {
	ctx := testkit.Context(t)
	owner, stranger := owners(t, ctx)
	svc := service.NewNoteService(ctx)

	// Straight into the table: 200 rows through the service would be 200 counts
	// and 200 inserts, and what is under test is the 201st.
	full := make([]models.Note, 0, 200)
	for i := range 200 {
		full = append(full, models.Note{
			BaseModel: models.NewBaseModel(),
			UserID:    owner,
			Title:     fmt.Sprintf("Note %03d", i),
			Body:      "x",
		})
	}
	require.NoError(t, ctx.DB().CreateInBatches(&full, 50).Error)

	_, err := svc.Create(owner, &service.CreatePayload{Title: "One too many", Body: "x"})

	require.NotNil(t, err)
	assert.Equal(t, http.StatusConflict, err.GetStatus())
	assert.Equal(t, "NOTE_LIMIT_REACHED", err.GetCode())

	t.Run("another account is unaffected", func(t *testing.T) {
		_, sErr := svc.Create(stranger, &service.CreatePayload{Title: "Fine", Body: "x"})

		assert.Nil(t, sErr, "the limit is per owner, not for the whole table")
	})
}
