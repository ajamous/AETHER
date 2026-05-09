module github.com/ajamous/aether/services/hsm-broker

go 1.22

require github.com/ajamous/aether/pkg/crypto v0.0.0-00010101000000-000000000000

require github.com/miekg/pkcs11 v1.1.2 // indirect

// In-workspace path resolution. go.work at the repo root pins these
// to the local module trees; these replace directives let `go mod
// tidy` (run outside the workspace) resolve cleanly too.
replace github.com/ajamous/aether/pkg/crypto => ../../pkg/crypto
