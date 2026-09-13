package service_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pskclub/mine-core-template/models"
	"github.com/pskclub/mine-core-template/modules/user/service"
	"github.com/pskclub/mine-core-template/testkit"
	core "github.com/pskclub/mine-core/v2"
)

func TestUserService_Create(t *testing.T) {
	ctx := testkit.Context(t)

	u, err := service.NewUserService(ctx).Create(&service.CreatePayload{
		Email:    "alice@example.com",
		FullName: "Alice",
	})

	require.Nil(t, err)
	assert.Equal(t, "alice@example.com", u.Email)
	assert.NotEmpty(t, u.ID, "the service assigns the id")
	assert.NotNil(t, u.CreatedAt)
}

func TestUserService_Find(t *testing.T) {
	ctx := testkit.Context(t)
	seeded := testkit.SeedUsers(t, ctx, "bob@example.com")

	t.Run("existing", func(t *testing.T) {
		u, err := service.NewUserService(ctx).Find(seeded[0].ID)

		require.Nil(t, err)
		assert.Equal(t, "bob@example.com", u.Email)
	})

	// A missing row is a 404, not a 500: the caller asked for something that is
	// not there, which is an answer rather than a failure.
	t.Run("missing", func(t *testing.T) {
		_, err := service.NewUserService(ctx).Find("00000000-0000-0000-0000-000000000000")

		require.NotNil(t, err)
		assert.Equal(t, http.StatusNotFound, err.GetStatus())
		assert.Equal(t, "NOT_FOUND", err.GetCode())
	})
}

func TestUserService_Update(t *testing.T) {
	ctx := testkit.Context(t)
	seeded := testkit.SeedUsers(t, ctx, "carol@example.com")
	svc := service.NewUserService(ctx)

	u, err := svc.Update(seeded[0].ID, &service.UpdatePayload{FullName: "Carol Updated"})

	require.Nil(t, err)
	assert.Equal(t, "Carol Updated", u.FullName)
	assert.Equal(t, "carol@example.com", u.Email, "update must not disturb other columns")

	t.Run("missing", func(t *testing.T) {
		_, err := svc.Update("00000000-0000-0000-0000-000000000000",
			&service.UpdatePayload{FullName: "Nobody"})

		require.NotNil(t, err)
		assert.Equal(t, http.StatusNotFound, err.GetStatus())
	})
}

func TestUserService_Delete(t *testing.T) {
	ctx := testkit.Context(t)
	seeded := testkit.SeedUsers(t, ctx, "dave@example.com")
	svc := service.NewUserService(ctx)

	require.Nil(t, svc.Delete(seeded[0].ID))

	_, err := svc.Find(seeded[0].ID)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusNotFound, err.GetStatus(), "a deleted user is gone from reads")

	// The row is only soft-deleted, so it is still recoverable and still holds
	// its unique email.
	var count int64
	require.NoError(t, ctx.DB().Unscoped().Model(&models.User{}).
		Where("id = ?", seeded[0].ID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "delete is soft: the row survives with deleted_at set")

	t.Run("missing", func(t *testing.T) {
		err := svc.Delete("00000000-0000-0000-0000-000000000000")

		require.NotNil(t, err)
		assert.Equal(t, http.StatusNotFound, err.GetStatus())
	})
}

func TestUserService_Pagination(t *testing.T) {
	ctx := testkit.Context(t)
	testkit.SeedUsers(t, ctx, "a@example.com", "b@example.com", "c@example.com")
	svc := service.NewUserService(ctx)

	t.Run("limits and counts", func(t *testing.T) {
		page, err := svc.Pagination(&core.PageOptions{Limit: 2, Page: 1})

		require.Nil(t, err)
		assert.Len(t, page.Items, 2, "limit applies to the page")
		assert.Equal(t, int64(3), page.Total, "total counts every match, not just this page")
	})

	t.Run("second page", func(t *testing.T) {
		page, err := svc.Pagination(&core.PageOptions{Limit: 2, Page: 2})

		require.Nil(t, err)
		assert.Len(t, page.Items, 1)
	})

	t.Run("q searches the full name", func(t *testing.T) {
		page, err := svc.Pagination(&core.PageOptions{Limit: 10, Page: 1, Q: "b@example"})

		require.Nil(t, err)
		require.Len(t, page.Items, 1)
		assert.Equal(t, "b@example.com", page.Items[0].Email)
	})

	t.Run("nil options are usable", func(t *testing.T) {
		page, err := svc.Pagination(nil)

		require.Nil(t, err)
		assert.Equal(t, int64(3), page.Total)
	})
}

func TestUserService_CreateMany(t *testing.T) {
	ctx := testkit.Context(t)
	svc := service.NewUserService(ctx)

	users, err := svc.CreateMany([]*service.CreatePayload{
		{Email: "one@example.com", FullName: "One"},
		{Email: "two@example.com", FullName: "Two"},
		{Email: "three@example.com", FullName: "Three"},
	})

	require.Nil(t, err)
	require.Len(t, users, 3)
	for _, u := range users {
		assert.NotEmpty(t, u.ID, "every user gets an id before insert")
	}

	page, err := svc.Pagination(&core.PageOptions{Limit: 10, Page: 1})
	require.Nil(t, err)
	assert.Equal(t, int64(3), page.Total, "all of them reached the database")
}

func TestUserService_CreateMany_empty(t *testing.T) {
	ctx := testkit.Context(t)

	users, err := service.NewUserService(ctx).CreateMany(nil)

	require.Nil(t, err)
	assert.Empty(t, users, "an empty batch is a no-op, not an error")
}

// A batch that fails partway must leave nothing behind, or a retry would
// duplicate whatever did get in.
//
// The payload spans more than one batch with the clash in a later one, so an
// earlier batch has already been written when the failure lands — a single
// failing INSERT would undo itself and prove nothing.
//
// This asserts the guarantee, not the mechanism: GORM wraps CreateInBatches in
// its own transaction unless SkipDefaultTransaction is set, so the test passes
// with or without the explicit one in CreateMany. It still catches the
// regression that matters — rewriting CreateMany as a loop of single Creates
// makes it fail.
func TestUserService_CreateMany_rollsBackOnFailure(t *testing.T) {
	ctx := testkit.Context(t)
	require.NoError(t, ctx.DB().Exec(
		`CREATE UNIQUE INDEX idx_users_email ON users(email) WHERE deleted_at IS NULL`).Error)

	svc := service.NewUserService(ctx)
	testkit.SeedUsers(t, ctx, "taken@example.com")

	const total = 60 // > createBatchSize, so this is at least two statements
	payloads := make([]*service.CreatePayload, 0, total)
	for i := range total {
		payloads = append(payloads, &service.CreatePayload{
			Email:    fmt.Sprintf("bulk-%02d@example.com", i),
			FullName: fmt.Sprintf("Bulk %02d", i),
		})
	}
	// lands in the second batch, after the first has already been written
	payloads[55].Email = "taken@example.com"

	_, err := svc.CreateMany(payloads)

	require.NotNil(t, err)
	assert.GreaterOrEqual(t, err.GetStatus(), http.StatusInternalServerError)

	// the first batch went in before the clash; the rollback must have undone it
	var count int64
	require.NoError(t, ctx.DB().Model(&models.User{}).
		Where("email LIKE ?", "bulk-%").Count(&count).Error)
	assert.Equal(t, int64(0), count, "a partial batch must not survive the failure")
}

// FindByEmail and CreateAccount are the entry points the auth module reaches
// through. They are covered here rather than only from auth's tests because they
// are this module's promise: changing what they return breaks a caller this
// package cannot see.
func TestUserService_accountEntryPoints(t *testing.T) {
	ctx := testkit.Context(t)
	svc := service.NewUserService(ctx)

	created, err := svc.CreateAccount("member@example.com", "Member", "a-bcrypt-hash")
	require.Nil(t, err)

	t.Run("the hash is stored as given", func(t *testing.T) {
		var row models.User
		require.NoError(t, ctx.DB().Where("id = ?", created.ID).First(&row).Error)
		assert.Equal(t, "a-bcrypt-hash", row.Password,
			"the module stores what auth hashed; it never hashes anything itself")
	})

	t.Run("FindByEmail returns the hash, which sign-in needs", func(t *testing.T) {
		found, fErr := svc.FindByEmail("member@example.com")

		require.Nil(t, fErr)
		assert.Equal(t, created.ID, found.ID)
		assert.Equal(t, "a-bcrypt-hash", found.Password)
	})

	t.Run("an unknown address is a 404, which is what tells auth to answer 401", func(t *testing.T) {
		_, fErr := svc.FindByEmail("nobody@example.com")

		require.NotNil(t, fErr)
		assert.Equal(t, http.StatusNotFound, fErr.GetStatus())
	})
}
