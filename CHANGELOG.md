# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Repository bootstrap: Apache 2.0 license, README with honest status table,
  CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, GOVERNANCE, MAINTAINERS, ROADMAP.
- Makefile with `build`, `test`, `lint`, `lab-up`, `lab-down`, `gen` targets.
- Go workspace stub (`go.work`).
- Linter and formatter configs (`.golangci.yml`, `.editorconfig`,
  `.prettierrc`).
- GitHub Actions CI workflow (build, test, lint, DCO check).
- CodeQL security scanning workflow.
- Release workflow stub.
- Issue and pull request templates.
- Documentation skeleton (MkDocs Material site).
- First five ADRs covering monorepo layout, language choice, HSM
  abstraction, lab/production cert mode switching, and license selection.
- ASN.1 toolchain scaffolding under `pkg/asn1/sgp22/` with `make gen`
  build step.

[Unreleased]: https://github.com/ajamous/aether/compare/HEAD...HEAD
