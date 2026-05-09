//go:build !cgo_asn1c

// This file documents the contract between Go-side types and the
// asn1c-generated C codec for SGP.22 types that cannot be expressed
// fully in Go's encoding/asn1.
//
// When such a type is needed, we will:
//
//   1. Add the .asn module under pkg/asn1/sgp22/modules/
//   2. Run `make gen` to produce C in pkg/asn1/sgp22/generated/c/
//   3. Add a Go file in this package guarded by `//go:build cgo_asn1c`
//      that wraps the C entry points via cgo
//   4. Provide pure-Go fallback types in this package guarded by
//      `//go:build !cgo_asn1c` so the package still builds without cgo
//      for development environments that don't have asn1c installed
//
// At Phase 0 we have no such type yet. This file exists to make the
// build-tag pattern visible and to keep the package free of surprise
// cgo dependencies during local development.

package sgp22
