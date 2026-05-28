module github.com/ajamous/aether/services/smdp-plus

go 1.26.0

require (
	github.com/ajamous/aether/pkg/certmgrclient v0.0.0-00010101000000-000000000000
	github.com/ajamous/aether/pkg/hsmclient v0.0.0-00010101000000-000000000000
	github.com/ajamous/aether/pkg/pbclient v0.0.0-00010101000000-000000000000
	github.com/ajamous/aether/pkg/saip v0.0.0-00010101000000-000000000000
	github.com/jackc/pgx/v5 v5.9.2
)

require (
	github.com/ajamous/aether/pkg/crypto v0.0.0-00010101000000-000000000000
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)

replace github.com/ajamous/aether/pkg/hsmclient => ../../pkg/hsmclient

replace github.com/ajamous/aether/pkg/pbclient => ../../pkg/pbclient

replace github.com/ajamous/aether/pkg/certmgrclient => ../../pkg/certmgrclient

replace github.com/ajamous/aether/pkg/crypto => ../../pkg/crypto

replace github.com/ajamous/aether/pkg/saip => ../../pkg/saip
