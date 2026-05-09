// Package v1 holds the gateway's wire-shape OpenAPI spec, embedded
// at compile time so the binary is self-describing. The server
// reads `Spec` to handle GET /v1/openapi.yaml.
package v1

import _ "embed"

// Spec is the embedded OpenAPI 3.1 document for the gateway's
// /v1/* admin surface and /gsma/rsp2/* ES2+ surface.
//
//go:embed openapi.yaml
var Spec []byte

// SpecBytes returns the spec contents. Indirection in case future
// versions assemble the spec at startup (e.g. interpolating the
// listen address).
func SpecBytes() []byte { return Spec }
