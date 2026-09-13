package auth

import (
	"time"

	"github.com/pskclub/mine-core-template/modules/auth/handler"
	core "github.com/pskclub/mine-core/v2"
)

// purgeInterval is how often expired tokens are swept. Hourly is plenty: the
// rows are harmless, this only stops the table growing without limit.
//
// It sits here rather than with the retention rule in service/ because it is a
// schedule, not a policy — how often to look, not what to keep.
const purgeInterval = time.Hour

// Cron arms this module's scheduled work.
//
// A module owns its jobs for the same reason it owns its tables: this one
// deletes from access_tokens, and how long a token's row is kept is an auth
// decision. It is only called in a role that has a scheduler — an api replica
// runs the routes in auth.http.go and arms nothing, from this same module list.
//
// Job names are prefixed with the module. They are stable identifiers — they
// label runs in the job store, in the logs and in Sentry cron monitors — so the
// prefix is what stops two teams from picking "cleanup" and silently colliding.
func (*Module) Cron(sc *core.Scheduler) core.IError {
	return sc.AddByDuration("auth.purge-expired-tokens", purgeInterval, handler.PurgeExpiredTokens)
}
