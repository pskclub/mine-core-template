// Package testkit is the test setup every package shares: an isolated database,
// a fully wired server, and the fixtures more than one module needs.
//
// It exists so a module's test file starts at the first interesting line. Before
// it, each package repeated its own coretest setup — including a
// filepath.Join("..", "..") that had to be counted correctly per directory, and
// a model list that had to be updated in several places whenever a table was
// added.
//
// Imported only from _test files, so nothing here reaches a production binary.
package testkit

import (
	"net/http"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pskclub/mine-core-template/cmd"
	"github.com/pskclub/mine-core-template/models"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/coretest"
)

// allModels is what sqlite's AutoMigrate builds. A new table goes here and
// nowhere else — the postgres run reads the prisma migrations instead, and does
// not need to be told.
func allModels() []any {
	return []any{
		&models.User{},
		&models.AccessToken{},
		&models.Note{},
	}
}

// migrationsDir resolves the migrations from *this* file's location, so a caller
// never has to know how deep it sits. runtime.Caller returns the path this
// package was compiled from, which is the repository on any machine that runs
// the tests.
func migrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "prisma", "schema", "migrations")
}

func options(extra ...coretest.Option) []coretest.Option {
	// WithMigrations is what makes the postgres run worth having — it applies
	// the prisma migrations rather than guessing a schema from the structs, so a
	// constraint the code contradicts fails here instead of in staging.
	base := []coretest.Option{
		coretest.WithAutoMigrate(allModels()...),
		coretest.WithMigrations(migrationsDir()),
	}

	return append(base, extra...)
}

// Context returns a context on an isolated database: sqlite by default, the real
// postgres schema when TEST_DATABASE_URL is set.
func Context(t *testing.T, extra ...coretest.Option) core.IContext {
	t.Helper()

	return coretest.NewContext(t, options(extra...)...)
}

// App is Context's App, for the code that runs before any handler — the guard,
// which is handed an App because a request's context arrives per call.
func App(t *testing.T, extra ...coretest.Option) *core.App {
	t.Helper()

	return coretest.NewApp(t, options(extra...)...)
}

// Serve starts the real service on a real listener: every module, wired by the
// same function cmd uses.
//
// Whole-service rather than one module's routes on purpose. A module's routes
// are only half of what it promises — the other half is that the guard on them
// is the real one, and that its dependencies were satisfied at wiring time. A
// test that registered routes by hand would pass with a mistake in
// cmd.NewAPI still in place.
func Serve(t *testing.T, extra ...coretest.Option) (*coretest.Client, *core.App) {
	t.Helper()

	app := App(t, extra...)

	// cmd.Modules, not a module list of its own: assembling the same set the
	// service assembles is what makes a missing module or an unsatisfied
	// dependency fail here. A list written out again in this file would agree
	// with itself and with nothing else.
	mods, err := cmd.Modules(app)
	require.Nil(t, err, "the module set must assemble: a duplicate or malformed name fails here")

	return coretest.Serve(t, cmd.NewAPI(app, mods, nil)), app
}

// The account SignIn creates. A test that needs a *second* account registers one
// itself; these are for the common case of "some signed-in caller".
const (
	FixtureEmail    = "tester@example.com"
	FixturePassword = "correct-horse"
)

// SignIn registers an account over the API and returns a client carrying its
// token, for the routes that sit behind the guard.
//
// Register and login rather than an inserted row plus a hand-made token: those
// two endpoints are the only supported way to obtain one, and a test that forged
// a token would keep passing after the real path broke.
func SignIn(t *testing.T, c *coretest.Client) *coretest.Client {
	t.Helper()

	c.Post("/auth/register", map[string]any{
		"email":     FixtureEmail,
		"full_name": "Tester",
		"password":  FixturePassword,
	}).RequireStatus(http.StatusCreated)

	token, _ := c.Post("/auth/login", map[string]any{
		"email":    FixtureEmail,
		"password": FixturePassword,
	}).RequireStatus(http.StatusOK).Map()["token"].(string)
	require.NotEmpty(t, token, "login must return a token")

	return coretest.NewClient(t, c.BaseURL(),
		coretest.WithHeader("Authorization", "Bearer "+token))
}

// SeedUsers inserts n users and returns them, for tests that need existing rows.
//
// It writes through the database rather than through the user service: a test
// arranging its starting state should not depend on the code it is about to
// exercise. Note the password is a literal, not a bcrypt hash — a seeded user
// can be listed and fetched, but can never sign in. Tests that need to sign in
// register through the auth module instead.
func SeedUsers(t *testing.T, ctx core.IContext, emails ...string) []models.User {
	t.Helper()

	out := make([]models.User, 0, len(emails))
	for _, email := range emails {
		u := models.User{
			BaseModel: models.NewBaseModel(),
			Email:     email,
			FullName:  "Seed " + email,
			Password:  "x",
		}
		require.NoError(t, ctx.DB().Create(&u).Error)
		out = append(out, u)
	}

	return out
}
