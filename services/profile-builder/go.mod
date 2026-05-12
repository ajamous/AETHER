module github.com/ajamous/aether/services/profile-builder

go 1.26.0

require (
	github.com/ajamous/aether/pkg/saip v0.0.0-00010101000000-000000000000
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/ajamous/aether/pkg/saip => ../../pkg/saip
