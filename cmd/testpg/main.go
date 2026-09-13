// Command testpg runs the test suite against a real postgres that this process
// starts, owns and throws away — no docker, no daemon, nothing installed.
//
// This is what `make test-integration` runs.
//
// It exists because the suite had two tiers and a gap between them. `make test`
// is sqlite, which needs nothing and is a different database: no partial
// indexes, no UUID type, no ILIKE, and a binder loose enough to accept
// `SELECT count(*) ... ORDER BY name`, which is the exact shape core.Paginate
// builds and postgres rejects. The postgres tier answers all of that — it is
// also the only tier that runs the prisma migrations themselves, because
// testkit adds coretest.WithMigrations — but it only ran once somebody had
// stood a docker postgres up first. So in practice it was run rarely, and a
// schema postgres would refuse could reach main.
//
// So: fetch a real postgres binary once, run it on a private port for the
// length of one `go test` invocation, and delete the cluster afterwards. The
// suite needs no change to use it — testkit already switches to postgres when
// TEST_DATABASE_URL is set, and that is the only thing this hands over.
//
//	go run ./cmd/testpg                          # the whole suite
//	go run ./cmd/testpg ./modules/note/...       # one package tree
//	go run ./cmd/testpg -run TestNoteAPI_crud ./modules/note/...
//	go run ./cmd/testpg -pglog                   # and show the server log
//
// # Pointing it at a postgres you already have
//
// A TEST_DATABASE_URL already in the environment wins: this starts nothing and
// runs the suite against that. It is how the docker-compose postgres is still
// reachable, and how CI — which gets postgres as a service container — keeps
// working unchanged:
//
//	TEST_DATABASE_URL=postgres://my_user:my_password@localhost:5432/my_database?sslmode=disable go run ./cmd/testpg
//
// It deliberately does not build the App. Bootstrap opens the configured
// database — the developer's actual one. A tool whose whole job is to stand up
// a throwaway database must not connect to the real one on the way, so this
// imports nothing of ours at all.
package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	os.Exit(run())
}

// run is main with a return value, because a deferred shutdown does not survive
// os.Exit and leaving a postgres behind locks the next run out of its data
// directory.
func run() int {
	args, verbose := parseArgs(os.Args[1:])

	// An explicit one wins and nothing is started. Both because standing up a
	// second postgres to ignore it would be absurd, and because this is the
	// only way left to reach a postgres somebody else provisioned — the
	// docker-compose one, or CI's service container.
	if url := os.Getenv(envDatabaseURL); url != "" {
		fmt.Fprintf(os.Stderr, "testpg: %s is set — using it, starting nothing\n\n", envDatabaseURL)

		return goTest(args, url)
	}

	port, err := freePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: no free port: %v\n", err)
		return 1
	}

	runtimeDir, err := runtimePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: runtime dir: %v\n", err)
		return 1
	}
	defer os.RemoveAll(runtimeDir)

	logger := io.Discard
	if verbose {
		logger = os.Stderr
	}

	postgres := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		// The version docker-compose and deployment run. The library defaults
		// to a newer major, and a test tier a major version ahead of production
		// reports problems production does not have and misses one it does.
		Version(embeddedpostgres.V17).
		Port(port).
		Database(database).
		Username(user).
		Password(password).
		// initdb takes its encoding from the host locale, and on Windows that
		// is WIN1252 — under which any non-latin1 literal in a test fails to
		// bind with "no equivalent in encoding WIN1252" before an assertion
		// runs. Both are named explicitly so the cluster is identical on every
		// machine rather than merely working on Linux.
		Encoding("UTF8").
		Locale("C").
		RuntimePath(runtimeDir).
		StartParameters(map[string]string{
			// go test runs packages in parallel and each test takes two
			// connections (coretest opens an admin handle and a schema-scoped
			// one), so the stock 100 is reachable once enough packages are in
			// flight. A refusal here reads as a random test failure.
			"max_connections": "200",

			// The cluster is deleted at the end of this function, so durability
			// buys nothing and costs a fsync per commit on a suite that commits
			// constantly.
			"fsync":              "off",
			"synchronous_commit": "off",
			"full_page_writes":   "off",
		}).
		// The first run downloads ~30MB and then initdbs; the library's default
		// 15 seconds is not always enough for that on a cold cache.
		StartTimeout(3 * time.Minute).
		Logger(logger))

	fmt.Fprintf(os.Stderr, "testpg: starting postgres 17 on port %d\n", port)

	started := time.Now()
	if err := postgres.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "testpg: start: %v\n", err)
		if !verbose {
			fmt.Fprintln(os.Stderr, "testpg: re-run with -pglog to see the server log")
		}
		return 1
	}

	// Ctrl+C reaches go test too and it exits on its own; this is here so the
	// postgres it was talking to is stopped rather than orphaned, holding a
	// port and a data directory that the deferred cleanup above would then
	// delete out from under it.
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt)
	defer signal.Stop(interrupted)

	stopped := make(chan struct{})
	go func() {
		select {
		case <-interrupted:
			_ = postgres.Stop()
		case <-stopped:
		}
	}()

	fmt.Fprintf(os.Stderr, "testpg: ready in %s\n\n", time.Since(started).Round(time.Millisecond))

	code := goTest(args, dsn(port))

	close(stopped)
	if err := postgres.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "testpg: stop: %v\n", err)
		if code == 0 {
			code = 1
		}
	}

	return code
}

const (
	database = "golang_template_test"
	user     = "postgres"
	password = "postgres"

	// The variable testkit switches on. Spelled out rather than taken from
	// coretest.EnvDatabaseURL, because coretest imports testing and this is a
	// binary — and because a tool that hands the suite its database should not
	// need the suite's own test harness to name the handover.
	envDatabaseURL = "TEST_DATABASE_URL"
)

// goTest runs the suite with TEST_DATABASE_URL pointing at the cluster.
//
// -count=1 rather than relying on the test cache: the cache keys on the test
// binary and its inputs, and the database is neither, so a cached PASS from the
// sqlite tier would be reported as a postgres one.
func goTest(args []string, url string) int {
	cmd := exec.Command("go", append([]string{"test", "-count=1"}, args...)...)
	cmd.Env = append(os.Environ(), envDatabaseURL+"="+url)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			return exit.ExitCode()
		}

		fmt.Fprintf(os.Stderr, "testpg: go test: %v\n", err)
		return 1
	}

	return 0
}

// parseArgs splits our own one option out and passes everything else to go test
// untouched.
//
// The flag package cannot do this: it stops at the first argument it does not
// recognise, so `-run TestFoo ./modules/note/...` — a flag it has never heard
// of, in first position — is a usage error rather than something to forward.
// And -pglog rather than -v because -v is go test's, meaning verbose test
// output; taking it here would shadow the flag every Go developer reaches for
// first.
func parseArgs(argv []string) (args []string, verbose bool) {
	args = make([]string, 0, len(argv))
	for _, arg := range argv {
		if arg == "-pglog" || arg == "--pglog" {
			verbose = true
			continue
		}

		args = append(args, arg)
	}

	// A bare `go test` tests the current directory; the suite is the default
	// this tool exists for.
	if len(args) == 0 {
		args = []string{"./..."}
	}

	return args, verbose
}

func dsn(port uint32) string {
	return fmt.Sprintf("postgres://%s:%s@localhost:%d/%s?sslmode=disable", user, password, port, database)
}

// runtimePath is where the binaries are extracted and the cluster initialised:
// a fresh directory per run, so two copies of this tool — a watch loop and a
// terminal — do not initdb into the same data directory and find it locked. The
// download cache stays at the library's shared default, so the binaries are
// fetched once ever rather than once per run.
//
// It lives beside that cache under $HOME rather than in os.TempDir(), because
// initdb walks the path creating parents and gives up on a directory it may not
// create: where TEMP is C:\WINDOWS\TEMP — which is the machine default under
// some Windows profiles — it fails with
//
//	initdb: error: could not create directory "C:/WINDOWS/TEMP": File exists
//
// $HOME is writable by definition for the user running the tests, so this is
// the one location that needs no explaining on any of the three platforms.
func runtimePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	base := filepath.Join(home, ".embedded-postgres-go", "run")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}

	return os.MkdirTemp(base, "testpg-")
}

// freePort asks the OS for one and hands back the number. Binding to a fixed
// port would collide with the docker-compose postgres, or with a second copy of
// this tool.
func freePort() (uint32, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	_, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return 0, err
	}

	port, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		return 0, err
	}

	return uint32(port), nil
}
