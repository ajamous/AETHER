module github.com/ajamous/aether/services/audit

go 1.26.0

require (
	github.com/ajamous/aether/pkg/hsmclient v0.0.0-00010101000000-000000000000
	github.com/jackc/pgx/v5 v5.9.2
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)

// In-workspace path resolution. go.work at the repo root pins this to
// the local module tree for dev; this replace directive lets the
// Docker build (which intentionally omits go.work) resolve cleanly too.
replace github.com/ajamous/aether/pkg/hsmclient => ../../pkg/hsmclient
