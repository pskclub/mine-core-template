// Package apispec carries this service's own OpenAPI document into the binary.
//
// It is embedded rather than read from disk so the binary always describes
// exactly the code it was built from: there is no file to forget to deploy, and
// no way for a running service to serve a reference for a different version of
// itself.
//
// The document is generated — `make postman` writes it from the registered
// routes — and committed, because go:embed needs it to exist at compile time.
// CI checks that the committed copy still matches the routes (`make api-check`),
// so a route added without regenerating fails the pipeline rather than silently
// going missing from the reference.
package apispec

import _ "embed"

//go:embed openapi.generated.json
var Spec []byte
