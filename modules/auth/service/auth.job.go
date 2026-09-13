package service

import (
	"time"

	"github.com/pskclub/mine-core-template/modules/auth/store"
	core "github.com/pskclub/mine-core/v2"
)

// TokenRetention is how long an expired token's row is kept after it stops
// working.
//
// Not zero, and that is the point. ResolveToken answers TOKEN_EXPIRED — "sign in
// again" — only while the row is still there; once it is gone the same token
// answers UNAUTHORIZED, which reads to a client as "something is wrong" rather
// than "your session ended". A week is long enough that anyone coming back to a
// stale tab gets the useful answer.
//
// Exported because the job's handler is what reads the clock and subtracts it.
// The rule stays here, beside the code whose behaviour it explains; only the
// arithmetic is out there.
const TokenRetention = 7 * 24 * time.Hour

// PurgeTokensExpiredBefore hard-deletes every token that expired before cutoff
// and reports how many went.
//
// The cutoff is a parameter rather than read from the clock inside, so the job's
// one interesting property — that it removes what is past the retention window
// and nothing else — can be tested at a fixed instant instead of by waiting.
//
// Hard, not soft: a soft delete would leave exactly the rows this is meant to
// remove. Unscoped is implied by HardDelete, so tokens already soft-deleted by a
// logout are swept too.
func PurgeTokensExpiredBefore(ctx core.IContext, cutoff time.Time) (int64, core.IError) {
	count, err := store.AccessToken(ctx).Unscoped().
		Where("expires_at < ?", cutoff).Count()
	if err != nil {
		return 0, ctx.NewError(err, err)
	}
	if count == 0 {
		return 0, nil
	}

	if err := store.AccessToken(ctx).HardDelete("expires_at < ?", cutoff); err != nil {
		return 0, ctx.NewError(err, err)
	}

	return count, nil
}
