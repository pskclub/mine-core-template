package auth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pskclub/mine-core-template/modules/auth"
	"github.com/pskclub/mine-core-template/testkit"
	core "github.com/pskclub/mine-core/v2"
)

// The sweep is armed under a name prefixed with its module, so two teams cannot
// both pick "cleanup" and have one silently replace the other.
//
// Cron is called on the module rather than a registration function, which is the
// point of core.IModule: there is no way to arm this job without adding auth to
// the module list, and no way to add auth to the list without arming it.
func TestModule_Cron(t *testing.T) {
	sc, err := core.NewScheduler(testkit.App(t))
	require.Nil(t, err)

	// the dependency is unused by Cron — only Routes reads it
	m := auth.New(nil)

	require.Nil(t, m.Cron(sc))

	// arming the same name twice is what a collision would look like
	assert.NotNil(t, m.Cron(sc),
		"a duplicate job name must be refused, not silently overwrite")
}
