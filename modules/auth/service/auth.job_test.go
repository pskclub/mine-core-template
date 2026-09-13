package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pskclub/mine-core-template/models"
	"github.com/pskclub/mine-core-template/modules/auth/service"
	"github.com/pskclub/mine-core-template/testkit"
	core "github.com/pskclub/mine-core/v2"
)

// token puts a row in the table directly, expiring at the given instant. Going
// through Login would tie every case to TokenTTL, and what is under test is the
// cutoff, not how a token is issued.
func token(t *testing.T, ctx core.IContext, digest string, expires time.Time, loggedOut bool) {
	t.Helper()

	row := models.AccessToken{
		BaseModel: models.NewBaseModel(),
		UserID:    "00000000-0000-0000-0000-000000000001",
		TokenHash: digest,
		ExpiresAt: &expires,
	}
	require.NoError(t, ctx.DB().Create(&row).Error)

	if loggedOut {
		require.NoError(t, ctx.DB().Where("id = ?", row.ID).
			Delete(&models.AccessToken{}).Error)
	}
}

// live counts the rows still present, soft-deleted ones included — the job hard
// deletes, so anything it touched is gone from here too.
func live(t *testing.T, ctx core.IContext) int64 {
	t.Helper()

	var count int64
	require.NoError(t, ctx.DB().Unscoped().Model(&models.AccessToken{}).Count(&count).Error)

	return count
}

func TestPurgeTokensExpiredBefore(t *testing.T) {
	ctx := testkit.Context(t)
	now := time.Now().UTC()

	token(t, ctx, "still-valid", now.Add(time.Hour), false)
	token(t, ctx, "expired-recently", now.Add(-time.Hour), false)
	token(t, ctx, "expired-long-ago", now.Add(-30*24*time.Hour), false)

	removed, err := service.PurgeTokensExpiredBefore(ctx, now.Add(-7*24*time.Hour))

	require.Nil(t, err)
	assert.Equal(t, int64(1), removed)
	assert.Equal(t, int64(2), live(t, ctx))

	// The recently expired row is deliberately kept: while it exists,
	// ResolveToken can answer TOKEN_EXPIRED — "sign in again" — instead of the
	// bare UNAUTHORIZED it gives once the row is gone.
	var kept models.AccessToken
	require.NoError(t, ctx.DB().Where("token_hash = ?", "expired-recently").First(&kept).Error)
}

// A logged-out token is soft-deleted, so it is exactly the row a soft delete
// would fail to clean up. The job hard deletes for that reason.
func TestPurgeTokensExpiredBefore_sweepsLoggedOutRows(t *testing.T) {
	ctx := testkit.Context(t)
	now := time.Now().UTC()

	token(t, ctx, "logged-out-and-old", now.Add(-30*24*time.Hour), true)

	removed, err := service.PurgeTokensExpiredBefore(ctx, now.Add(-7*24*time.Hour))

	require.Nil(t, err)
	assert.Equal(t, int64(1), removed)
	assert.Equal(t, int64(0), live(t, ctx), "the row is gone for good, not soft-deleted again")
}

func TestPurgeTokensExpiredBefore_nothingToDo(t *testing.T) {
	ctx := testkit.Context(t)
	now := time.Now().UTC()

	token(t, ctx, "still-valid", now.Add(time.Hour), false)

	removed, err := service.PurgeTokensExpiredBefore(ctx, now.Add(-7*24*time.Hour))

	require.Nil(t, err)
	assert.Equal(t, int64(0), removed)
	assert.Equal(t, int64(1), live(t, ctx))
}
