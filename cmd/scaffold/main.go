// Command scaffold writes a new module: the nine files every module has, in the
// three layers and wired the way modules/note is.
//
//	make new-module name=order
//
// It exists because consistency is what makes a large codebase readable, and
// consistency by hand across a dozen people is a losing game. The thirtieth
// module should look like the first, and the way to get that is for nobody to
// type it out.
//
// It only writes under modules/. The four things it cannot do — the prisma
// schema, the model, the error codes, and the line in cmd/modules.go — are
// printed as a checklist, because each one is a decision rather than a
// transcription.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

func main() {
	name := flag.String("name", "", "module name, singular and lower case (order, invoice, api_key)")
	flag.Parse()

	if err := run(*name); err != nil {
		fmt.Fprintln(os.Stderr, "scaffold:", err)
		os.Exit(1)
	}
}

// names is every spelling of the module name the templates need. Deriving them
// once here means a template never has to do string surgery.
type names struct {
	Package  string // order      — directory and package name
	Type     string // Order      — exported prefix: OrderHandler, IOrderService
	Var      string // order      — local variable and unexported prefix
	Model    string // Order      — models.Order
	Route    string // orders     — the first path segment
	Table    string // orders     — the prisma model, for the checklist
	FilePfx  string // order      — file name prefix: order.service.go
	Singular string // order      — for prose in the generated comments
	Module   string // github.com/acme/orders — this repository's import path, from go.mod
}

func run(name string) error {
	if name == "" {
		return errors.New("missing -name (try: make new-module name=order)")
	}
	if err := validate(name); err != nil {
		return err
	}

	n := derive(name)
	mod, err := readModulePath("go.mod")
	if err != nil {
		return err
	}
	n.Module = mod
	dir := filepath.Join("modules", n.Package)

	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%s already exists — pick another name, or delete it first", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	for _, f := range files {
		path := filepath.Join(dir, f.dir, n.FilePfx+f.suffix)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := write(path, f.tmpl, n); err != nil {
			return err
		}
		fmt.Println("created", path)
	}

	fmt.Print(checklist(n))

	return nil
}

// readModulePath reads this repository's module path from go.mod. The templates
// import the project's own packages through it, so a renamed module — gonew, a
// fork, "Use this template" — scaffolds imports that compile, rather than ones
// that point back at the template it came from.
func readModulePath(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s — run scaffold from the repository root (make new-module name=...): %w", path, err)
	}
	for line := range strings.Lines(string(data)) {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`), nil
		}
	}

	return "", fmt.Errorf("%s has no module directive", path)
}

// validate rejects a name that would produce a package Go will not accept, or
// one that reads as a plural. Modules are named for one of the thing they hold.
func validate(name string) error {
	if name != strings.ToLower(name) {
		return fmt.Errorf("name must be lower case: %q", name)
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && r != '_' {
			return fmt.Errorf("name must be letters and underscores only: %q", name)
		}
	}
	if strings.HasSuffix(name, "s") {
		return fmt.Errorf("name should be singular: %q — the route is pluralised for you", name)
	}

	return nil
}

func derive(name string) names {
	exported := camel(name)

	return names{
		Package:  name,
		Type:     exported,
		Var:      strings.ToLower(name[:1]) + camel(name)[1:],
		Model:    exported,
		Route:    name + "s",
		Table:    name + "s",
		FilePfx:  name,
		Singular: strings.ReplaceAll(name, "_", " "),
	}
}

// camel turns api_key into ApiKey.
func camel(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}

	return strings.Join(parts, "")
}

func write(path, body string, n names) error {
	tmpl, err := template.New(filepath.Base(path)).Parse(body)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}

	if err := tmpl.Execute(f, n); err != nil {
		_ = f.Close()
		return err
	}

	// Close is checked rather than deferred: a write that fails on flush would
	// otherwise leave a truncated file behind and report success.
	return f.Close()
}

func checklist(n names) string {
	var b strings.Builder

	fmt.Fprintf(&b, `
The module compiles once these four are in place. Each is a decision, which is
why it is not generated:

  1. prisma/schema/%s.prisma
       model %s { id String @id @default(uuid()) @db.Uuid ... }
       Then run the migration yourself — nothing in Go touches the schema.

  2. models/%s.model.go
       type %s struct { BaseModel; ... }
       func (%s) TableName() string { return "%s" }

  3. testkit/testkit.go
       add &models.%s{} to allModels(), so the sqlite suite builds the table

  4. cmd/modules.go
       add  %s.New() to the core.NewModules(...) list in Modules
       This is the only place that knows the module exists. Until the line is
       there nothing it registers is attached, and the generated tests fail.

Error codes, if this module needs any of its own, go in emsgs/%s.emsgs.go.

Scheduled work, if it has any, is armed by a Cron method in %s.jobs.go and runs a
core.ICronjobContext function in modules/%s/handler/%s.job.go — a job belongs to
the module whose rows it touches, and its entry point is a handler for the same
reason an HTTP one is: it binds, converts, calls the service and records the
result, so the service never learns what triggered it. See modules/auth.
`,
		n.Table, n.Table,
		n.Package, n.Model, n.Model, n.Table,
		n.Model,
		n.Package,
		n.Package,
		n.FilePfx, n.Package, n.FilePfx,
	)

	return b.String()
}

// file is one generated file: which directory inside the module it goes in, what
// it is called, and what it starts as.
type file struct {
	dir    string
	suffix string
	tmpl   string
}

// files is the shape of a module. A new one starts with all of these and deletes
// what it has no work for — modules/home is a single module file.
//
// The three directories are the layers, and each may only reach the one below:
// handler → service → store. The module's own directory holds the attachment
// points, one file per kind: <name>.module.go declares the module, <name>.http.go
// its routes, and a <name>.jobs.go or <name>.mq.go appears if it grows those.
// The split is so a file name stays true — the guarantee is that no attachment
// point lives outside this directory, not that there is only one file.
var files = []file{
	{dir: "", suffix: ".module.go", tmpl: tmplModule},
	{dir: "", suffix: ".http.go", tmpl: tmplHTTP},
	{dir: "handler", suffix: ".handler.go", tmpl: tmplHandler},
	{dir: "handler", suffix: ".request.go", tmpl: tmplRequest},
	{dir: "service", suffix: ".service.go", tmpl: tmplService},
	{dir: "service", suffix: ".dto.go", tmpl: tmplDTO},
	{dir: "service", suffix: ".service_test.go", tmpl: tmplServiceTest},
	{dir: "store", suffix: ".store.go", tmpl: tmplStore},
	{dir: "", suffix: ".module_test.go", tmpl: tmplHTTPTest},
}

const tmplModule = `// Package {{.Package}} owns the {{.Table}} table, and every read and write of it.
//
// Other modules never query that table. They call the module through its
// {{.FilePfx}}.api.go — which this module does not have yet, because nothing else
// needs it. Add one when something does; see modules/user/user.api.go.
//
// This directory holds what the module attaches to the service and nothing
// else, one file per kind of attachment. The work is in handler/, service/ and
// store/, which nothing outside modules/{{.Package}} imports.
package {{.Package}}

// Module is everything {{.Package}} attaches to the service.
//
// Name is all core.IModule requires. The rest are optional interfaces, so this
// grows one method at a time as the module gains work, each in the file named
// for it — which is what stops a feature from being half-registered, since
// there is nowhere else for any of them to go:
//
//	Routes(e *core.Server)                    {{.FilePfx}}.http.go
//	Jobs(reg *core.JobRegistry) core.IError   {{.FilePfx}}.jobs.go — triggered on demand
//	Cron(sc *core.Scheduler) core.IError      {{.FilePfx}}.jobs.go — armed only where there is a scheduler
//	Consumers(c core.IMQConsumer)             {{.FilePfx}}.mq.go
//	HealthChecks() []core.HealthCheck         dependencies the framework cannot see
//	Start(app) / Stop(ctx)                    background work this module owns
type Module struct{}

func New() *Module { return &Module{} }

func (*Module) Name() string { return "{{.Package}}" }
`

const tmplHTTP = `package {{.Package}}

import (
	"{{.Module}}/middlewares"
	"{{.Module}}/modules/{{.Package}}/handler"
	core "github.com/pskclub/mine-core/v2"
)

// Routes registers every {{.Singular}} route, and this file holds nothing else —
// so "which URLs does this module serve" is answered by opening one file.
//
// Paths are written out in full and the middleware is applied per route, so a
// path seen in a log can be grepped for and lands here, and whether a route is
// protected is answerable on the line itself.
func (*Module) Routes(e *core.Server) {
	c := &handler.{{.Type}}Handler{}

	e.GET("/{{.Route}}", c.Pagination, middlewares.AuthRequire(e))
	e.GET("/{{.Route}}/:id", c.Find, middlewares.AuthRequire(e))
	e.POST("/{{.Route}}", c.Create, middlewares.AuthRequire(e))
	e.DELETE("/{{.Route}}/:id", c.Delete, middlewares.AuthRequire(e))
}
`

const tmplHandler = `// Package handler is the {{.Singular}} module's HTTP edge: what arrives, how it
// is validated, and what is written back.
//
// It knows about requests and status codes; the service it calls knows about
// neither. Nothing outside modules/{{.Package}} imports this.
package handler

import (
	"net/http"

	"{{.Module}}/modules/{{.Package}}/service"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/utils"
)

// {{.Type}}Handler holds the module's HTTP handlers.
//
// A handler does four things and nothing else: bind, convert, call, render.
// Any line here that decides something belongs in the service, where a job can
// reach it too.
type {{.Type}}Handler struct {
}

func (m {{.Type}}Handler) Pagination(c core.IHTTPContext) error {
	// The allowlist is what makes order_by safe — a column outside it is dropped
	// instead of reaching SQL.
	opts := c.GetPageOptionsWithAllowed("created_at")

	res, err := service.New{{.Type}}Service(c).Pagination(opts)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (m {{.Type}}Handler) Find(c core.IHTTPContext) error {
	{{.Var}}, err := service.New{{.Type}}Service(c).Find(c.Param("id"))
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, {{.Var}})
}

func (m {{.Type}}Handler) Create(c core.IHTTPContext) error {
	input := &CreateRequest{}
	if err := c.BindWithValidate(input); err != nil {
		return err
	}

	payload, _ := utils.Copy[service.CreatePayload](input)

	{{.Var}}, err := service.New{{.Type}}Service(c).Create(&payload)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, {{.Var}})
}

func (m {{.Type}}Handler) Delete(c core.IHTTPContext) error {
	if err := service.New{{.Type}}Service(c).Delete(c.Param("id")); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
`

const tmplRequest = `package handler

import (
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/valid"
)

// A request states what has to be true of the payload itself, and nothing else.
// Anything that depends on rows — "does this name already exist for this
// account" — belongs in the service, which can see them.
//
// Error codes and their messages live in emsgs, never inline here.
type CreateRequest struct {
	// TODO: the fields this endpoint accepts, as pointers with json tags
}

func (r *CreateRequest) Valid(ctx core.IContext) core.IError {
	v := valid.New(ctx)

	// TODO: v.Str("name", r.Name).Required().Length(1, 100)

	return v.Error()
}
`

const tmplService = `// Package service holds the {{.Singular}} module's business rules.
//
// It is imported by this module's handler and by modules/{{.Package}} itself, and
// by nothing else.
package service

import (
	"{{.Module}}/models"
	"{{.Module}}/modules/{{.Package}}/store"
	"{{.Module}}/repo"
	core "github.com/pskclub/mine-core/v2"
)

// I{{.Type}}Service is this module's whole surface — to its own handler and,
// through {{.FilePfx}}.api.go, to every other module. Nothing outside this module
// touches the {{.Table}} table except through one of these methods.
type I{{.Type}}Service interface {
	Create(input *CreatePayload) (*models.{{.Model}}, core.IError)
	Find(id string) (*models.{{.Model}}, core.IError)
	Pagination(pageOptions *core.PageOptions) (*core.Page[models.{{.Model}}], core.IError)
	Delete(id string) core.IError
}

type {{.Var}}Service struct {
	ctx core.IContext
}

// New{{.Type}}Service takes any core.IContext — the same service runs unchanged in
// an HTTP handler, a job or a test.
func New{{.Type}}Service(ctx core.IContext) I{{.Type}}Service {
	return &{{.Var}}Service{ctx: ctx}
}

func (s {{.Var}}Service) Create(input *CreatePayload) (*models.{{.Model}}, core.IError) {
	{{.Var}} := &models.{{.Model}}{
		BaseModel: models.NewBaseModel(),
		// TODO: copy the fields from input
	}

	// NewError(err, err) keeps the repository's own status and code (404 stays a
	// 404) while reporting 5xx to Sentry here, where the context still knows the
	// user and the request.
	if err := store.{{.Model}}(s.ctx).Create({{.Var}}); err != nil {
		return nil, s.ctx.NewError(err, err)
	}

	return s.Find({{.Var}}.ID)
}

func (s {{.Var}}Service) Find(id string) (*models.{{.Model}}, core.IError) {
	{{.Var}}, err := store.{{.Model}}(s.ctx).FindOne("id = ?", id)
	if err != nil {
		return nil, s.ctx.NewError(err, err)
	}

	return {{.Var}}, nil
}

func (s {{.Var}}Service) Pagination(pageOptions *core.PageOptions) (*core.Page[models.{{.Model}}], core.IError) {
	opts := repo.DefaultOrder(pageOptions, "created_at DESC")

	page, err := store.{{.Model}}(s.ctx).Pagination(opts)
	if err != nil {
		return nil, s.ctx.NewError(err, err)
	}

	return page, nil
}

func (s {{.Var}}Service) Delete(id string) core.IError {
	if _, err := s.Find(id); err != nil {
		return err
	}

	if err := store.{{.Model}}(s.ctx).Delete("id = ?", id); err != nil {
		return s.ctx.NewError(err, err)
	}

	return nil
}
`

const tmplDTO = `package service

// A payload is what the service takes, and it is deliberately not the request
// struct. The request has pointers and binding tags because it describes what
// arrived over HTTP; the payload describes what the operation needs. Keeping
// them apart is what lets a job or another module call the same service without
// constructing a fake HTTP request.

type CreatePayload struct {
	// TODO: the fields Create needs
}
`

const tmplStore = `// Package store is how the {{.Singular}} module reaches its own table, and the
// only package that does.
//
// {{.Table}} belongs to this module. Nothing outside modules/{{.Package}} may
// import this package — arch enforces it — so another module that needs
// a row asks through I{{.Type}}Service and gets only what this module chose to
// offer. That is what keeps a column changeable a year from now.
package store

import (
	"{{.Module}}/models"
	core "github.com/pskclub/mine-core/v2"
	"github.com/pskclub/mine-core/v2/repository"
)

// {{.Model}} opens the {{.Table}} repository on ctx. The context (deadline,
// cancellation, trace) is bound once here, so no query method takes one again.
func {{.Model}}(ctx core.IContext) *repository.Repo[models.{{.Model}}] {
	return repository.New[models.{{.Model}}](ctx)
}
`

const tmplServiceTest = `package service_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"{{.Module}}/modules/{{.Package}}/service"
	"{{.Module}}/testkit"
)

func Test{{.Type}}Service_Find_missing(t *testing.T) {
	ctx := testkit.Context(t)

	_, err := service.New{{.Type}}Service(ctx).Find("00000000-0000-0000-0000-000000000000")

	require.NotNil(t, err)
	assert.Equal(t, http.StatusNotFound, err.GetStatus(),
		"a missing row is an answer, not a failure")
}

// TODO: a test per rule this module promises. See modules/note for the shape —
// arrange with testkit, call the service directly, assert on the error's status
// and code rather than on its text.
`

const tmplHTTPTest = `package {{.Package}}_test

import (
	"net/http"
	"testing"

	"{{.Module}}/testkit"
	"github.com/pskclub/mine-core/v2/coretest"
)

// serve starts the real service and returns a signed-in client.
//
// The whole service, not this module's routes alone: the token lookup behind
// middlewares.AuthRequire is installed by cmd.NewAPI, so a test that
// registered these routes by hand would be testing a wiring that does not ship.
func serve(t *testing.T) *coretest.Client {
	t.Helper()

	anon, _ := testkit.Serve(t)

	return testkit.SignIn(t, anon)
}

// middlewares.AuthRequire(e) is written on every route by hand, so this is what proves none of
// them was missed.
func Test{{.Type}}API_requiresAuthentication(t *testing.T) {
	c := serve(t)
	anon := coretest.NewClient(t, c.BaseURL())

	anon.Get("/{{.Route}}").RequireStatus(http.StatusUnauthorized)
	anon.Post("/{{.Route}}", map[string]any{}).RequireStatus(http.StatusUnauthorized)
	anon.Get("/{{.Route}}/00000000-0000-0000-0000-000000000000").
		RequireStatus(http.StatusUnauthorized)
	anon.Delete("/{{.Route}}/00000000-0000-0000-0000-000000000000").
		RequireStatus(http.StatusUnauthorized)
}
`
