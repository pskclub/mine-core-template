package handler

import (
	"time"

	"github.com/pskclub/mine-core-template/modules/auth/service"
	core "github.com/pskclub/mine-core/v2"
)

// PurgeExpiredTokens removes tokens that expired long enough ago to be of no
// further use.
//
// Logout soft-deletes a row and expiry leaves it in place, so without this the
// table only ever grows: one row per sign-in, for the life of the service.
//
// It is a handler and not a service method for the same reason the HTTP ones
// are: it reads the trigger's input (here, the clock), converts it to what the
// operation takes, calls the service and records the result. Keeping that here
// is what lets service/ stay written against core.IContext alone — it never
// learns whether a request or a scheduler asked.
func PurgeExpiredTokens(c core.ICronjobContext) error {
	cutoff := time.Now().UTC().Add(-service.TokenRetention)

	removed, err := service.PurgeTokensExpiredBefore(c, cutoff)
	if err != nil {
		// Returned, not logged: the job runner logs a failed run with the job
		// name, the run id and the attempt, and reports it. A line here would be
		// the same failure written twice.
		return err
	}

	// c.Log() already carries job, run_id and attempt — passing them again only
	// prints them twice. Log what *this* job did, not what the runner knows.
	//
	// Every run writes this line, including the ones that deleted nothing. A job
	// that only speaks up when it acts is indistinguishable from a job that has
	// stopped running, which is the failure worth catching: count=0 every hour
	// is the proof it is alive.
	c.Log().Info("purged expired access tokens", "count", removed, "cutoff", cutoff)

	return nil
}
