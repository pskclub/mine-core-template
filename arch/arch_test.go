// Package arch enforces the import rules the layout depends on.
//
// Go already refuses an import cycle, which covers the mistakes that are
// immediately fatal. It says nothing about the ones that rot slowly: a model
// reaching back into a module, a module quietly importing a sibling, a shared
// package learning about a feature. Each is legal, each compiles, and each is
// the step that turns a set of modules back into one tangle.
//
// So the rules are a test. It runs under `make test` with everything else, needs
// no tool anyone has to install, and fails in CI on the commit that breaks it
// rather than at the code review that did not notice.
//
// Only non-test imports are examined. A module's external test package may
// import a sibling — modules/auth's tests wire the real user service, exactly as
// cmd.NewAPI does — and that is not a dependency of the shipped binary.
package arch

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// modulePath is read from go.mod rather than written here, so the rules keep
// working after the module is renamed: gonew and a fork both rewrite imports,
// and neither touches a string constant.
var modulePath = readModulePath()

// rule is one layer's promise about what it will not reach for.
type rule struct {
	// name appears in the failure, so it explains itself.
	name string
	// applies reports whether the rule governs this package.
	applies func(pkg string) bool
	// forbids reports whether importing this package breaks the rule.
	forbids func(pkg, imported string) bool
	// because is the failure message: why the rule exists, not what it does.
	because string
}

var rules = []rule{
	{
		name:    "the shared kernel depends on nothing",
		applies: inAny("models", "consts"),
		forbids: func(_, imported string) bool { return owned(imported) },
		because: "These hold types, not behaviour, and everything depends on them.\n" +
			"A dependency back is what turns a shared type into a shared tangle:\n" +
			"the package can no longer be read, moved or compiled on its own.",
	},
	{
		name:    "shared infrastructure knows no module",
		applies: inAny("repo", "emsgs", "helpers", "middlewares"),
		forbids: func(_, imported string) bool {
			return under(imported, "modules") || under(imported, "cmd") ||
				imported == modulePath+"/testkit"
		},
		because: "These sit below every module and are imported by all of them.\n" +
			"Reaching up into one makes every module that uses them depend on it\n" +
			"by accident, and closes a cycle the next time that module grows.\n" +
			"middlewares is the sharpest case: every module's routes import it,\n" +
			"so importing a module back would stop the build outright. That is\n" +
			"why the token lookup is handed to it at startup instead — see\n" +
			"middlewares/auth.go and cmd.Modules.",
	},
	{
		name:    "no module imports another module",
		applies: under2("modules"),
		forbids: func(pkg, imported string) bool {
			return under(imported, "modules") && moduleOf(imported) != moduleOf(pkg)
		},
		because: "A module declares what it needs as an interface of its own, and\n" +
			"cmd.Modules supplies it. Importing a sibling directly works\n" +
			"exactly once: the second such import, in the other direction, stops\n" +
			"compiling and has to be untangled under pressure.\n" +
			"See modules/auth/service/auth.deps.go for the pattern that avoids it.",
	},
	{
		name:    "a module does not know what wires it",
		applies: under2("modules"),
		forbids: func(_, imported string) bool { return under(imported, "cmd") },
		because: "Composition points one way. cmd.Modules assembles the set and\n" +
			"core.RunModules attaches it to whatever this role has; both call\n" +
			"into modules, and neither is called back. A module that reached for\n" +
			"the thing assembling it could not be tested, reused or removed on\n" +
			"its own — and it is why registration is a value passed to\n" +
			"core.NewModules rather than an init() nobody can override.",
	},
	{
		name: "a module is entered through its top-level package only",
		applies: func(pkg string) bool {
			return owned(pkg) && !under(pkg, "modules")
		},
		forbids: func(_, imported string) bool {
			return under(imported, "modules") && strings.Contains(moduleSubPath(imported), "/")
		},
		because: "handler/, service/ and store/ are how a module is arranged\n" +
			"internally, and rearranging them should be an edit inside one\n" +
			"directory. An import that reaches past modules/<name> makes the\n" +
			"arrangement part of the contract, and every such import is another\n" +
			"caller to find before a file can move.\n" +
			"What a module offers others goes in its <name>.api.go, which\n" +
			"forwards — see modules/user/user.api.go.",
	},
	{
		name:    "test-only code stays out of the binary",
		applies: func(string) bool { return true },
		forbids: func(_, imported string) bool {
			return imported == modulePath+"/testkit"
		},
		because: "testkit builds databases and servers for tests. Imported outside a\n" +
			"_test file it would ship inside the binary, along with testing itself.",
	},
}

func TestImportRules(t *testing.T) {
	for _, pkg := range listPackages(t) {
		for _, r := range rules {
			if !r.applies(pkg.ImportPath) {
				continue
			}

			for _, imported := range pkg.Imports {
				if !r.forbids(pkg.ImportPath, imported) {
					continue
				}

				t.Errorf("\n%s\n\n  %s\n  imports %s\n\n%s\n",
					r.name, short(pkg.ImportPath), short(imported), indent(r.because))
			}
		}
	}
}

type listedPackage struct {
	ImportPath string
	Imports    []string
}

// listPackages asks the toolchain what every package in this repository imports.
// `go list` is the authority — it resolves build tags and generated files, which
// parsing the source by hand would not.
func listPackages(t *testing.T) []listedPackage {
	t.Helper()

	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = repoRoot()

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}

	var pkgs []listedPackage
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var p listedPackage
		if err := dec.Decode(&p); err != nil {
			t.Fatalf("decoding go list output: %v", err)
		}
		pkgs = append(pkgs, p)
	}

	if len(pkgs) == 0 {
		t.Fatal("go list returned no packages; the rules would pass vacuously")
	}

	return pkgs
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..")
}

func readModulePath() string {
	data, err := os.ReadFile(filepath.Join(repoRoot(), "go.mod"))
	if err != nil {
		panic("arch: reading go.mod: " + err.Error())
	}
	for line := range strings.Lines(string(data)) {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`)
		}
	}
	panic("arch: no module directive in go.mod")
}

// owned reports whether an import path belongs to this repository. Everything
// else is a third-party dependency, which no rule here has an opinion about.
func owned(path string) bool {
	return path == modulePath || strings.HasPrefix(path, modulePath+"/")
}

func under(path, dir string) bool {
	return path == modulePath+"/"+dir || strings.HasPrefix(path, modulePath+"/"+dir+"/")
}

// inAny matches a top-level package by name: "models" matches models, not
// modules/user/models.
func inAny(names ...string) func(string) bool {
	return func(path string) bool {
		for _, name := range names {
			if path == modulePath+"/"+name {
				return true
			}
		}
		return false
	}
}

func under2(dir string) func(string) bool {
	return func(path string) bool { return under(path, dir) }
}

// moduleSubPath is a package's path below modules/ — "note" for the module's own
// package, "note/handler" for one of its parts.
func moduleSubPath(path string) string {
	return strings.TrimPrefix(path, modulePath+"/modules/")
}

// moduleOf names the module a package belongs to, so modules/note and
// modules/note/handler count as the same one — a module splitting itself into
// directories is not a cross-module import.
func moduleOf(path string) string {
	name, _, _ := strings.Cut(moduleSubPath(path), "/")

	return name
}

func short(path string) string {
	return strings.TrimPrefix(path, modulePath+"/")
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}
