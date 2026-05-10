# Aether top-level Makefile.
#
# Conventions:
#   - Every target prints a one-line description in `make help`.
#   - Targets that require external tools fail fast with an actionable
#     install hint instead of a cryptic shell error.
#   - The 60-second `make lab-up` is the most important target in this
#     file. Anything that slows it down needs a really good reason.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

# ---- Versions / paths -------------------------------------------------------

GO          ?= go
NODE        ?= node
NPM         ?= npm
PYTHON      ?= python3
DOCKER      ?= docker
COMPOSE     ?= docker compose
ASN1C       ?= asn1c

REPO_ROOT   := $(shell git rev-parse --show-toplevel 2>/dev/null || pwd)
LAB_FILE    := deployments/docker-compose/lab.yml

# ---- Helpers ----------------------------------------------------------------

# require-tool: fail with an install hint if the binary is missing.
define require-tool
	@command -v $(1) >/dev/null 2>&1 || { \
	  echo >&2 "error: '$(1)' is required but not on PATH."; \
	  echo >&2 "       hint: $(2)"; \
	  exit 1; \
	}
endef

# ---- Top-level targets ------------------------------------------------------

.PHONY: help
help: ## Show this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage: make <target>\n\nTargets:\n"} \
	  /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } \
	  /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Build and test

.PHONY: build
build: ## Build all Go modules in the workspace.
	$(call require-tool,$(GO),install Go 1.25.10+ from https://go.dev/dl/)
	@for d in $$(find . -name go.mod -not -path './vendor/*' -exec dirname {} \;); do \
	  echo "==> build $$d"; \
	  (cd "$$d" && $(GO) build ./...) || exit $$?; \
	done

.PHONY: test
test: ## Run unit tests in every Go module.
	$(call require-tool,$(GO),install Go 1.25.10+)
	@for d in $$(find . -name go.mod -not -path './vendor/*' -exec dirname {} \;); do \
	  echo "==> test $$d"; \
	  (cd "$$d" && $(GO) test ./...) || exit $$?; \
	done

.PHONY: lint
lint: ## Run all linters (Go, JS, YAML).
	$(call require-tool,golangci-lint,go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

.PHONY: govulncheck
govulncheck: ## Scan every Go module for reachable known-vulnerability call paths.
	$(call require-tool,govulncheck,go install golang.org/x/vuln/cmd/govulncheck@latest)
	@fail=0; \
	for d in $$(find . -name go.mod -not -path './vendor/*' -not -path './ui/admin/node_modules/*' -exec dirname {} \;); do \
	  echo "==> govulncheck $$d"; \
	  out=$$(cd "$$d" && govulncheck ./... 2>&1) || rc=$$?; \
	  rc=$${rc:-0}; \
	  if [ $$rc -ne 0 ] && ! echo "$$out" | grep -qE 'no packages|matched no'; then \
	    echo "$$out"; fail=1; \
	  fi; \
	  unset rc; \
	done; \
	exit $$fail

.PHONY: tidy
tidy: ## Tidy Go modules across the workspace.
	$(call require-tool,$(GO),install Go 1.25.10+)
	$(GO) work sync || true
	for d in $$(find . -name go.mod -not -path './vendor/*' -exec dirname {} \;); do \
	  (cd "$$d" && $(GO) mod tidy); \
	done

##@ Code generation

.PHONY: gen
gen: gen-asn1 ## Run all code generators.

.PHONY: gen-asn1
gen-asn1: ## Regenerate ASN.1 bindings for SGP.22 modules.
	@echo "==> Regenerating SGP.22 ASN.1 bindings"
	$(MAKE) -C pkg/asn1/sgp22 gen

##@ Local lab

.PHONY: lab-up
lab-up: ## Start the local lab stack (target: <60s on a fresh clone).
	$(call require-tool,$(DOCKER),install Docker Engine 24+)
	@if [ ! -f $(LAB_FILE) ]; then \
	  echo "lab compose file not yet present at $(LAB_FILE)."; \
	  echo "this target will become functional in Phase 1."; \
	  exit 0; \
	fi
	$(COMPOSE) -f $(LAB_FILE) up -d --wait

.PHONY: lab-down
lab-down: ## Tear down the local lab stack.
	$(call require-tool,$(DOCKER),install Docker Engine 24+)
	@if [ ! -f $(LAB_FILE) ]; then exit 0; fi
	$(COMPOSE) -f $(LAB_FILE) down -v

.PHONY: lab-logs
lab-logs: ## Tail logs from the local lab stack.
	@if [ ! -f $(LAB_FILE) ]; then exit 0; fi
	$(COMPOSE) -f $(LAB_FILE) logs -f --tail=100

.PHONY: lab-test
lab-test: ## Run the lab smoke tests against a running stack.
	$(call require-tool,$(GO),install Go 1.25.10+)
	cd test/e2e && $(GO) test -tags=lab ./...

.PHONY: conformance
conformance: ## Run the SGP.23 conformance suite.
	$(call require-tool,$(GO),install Go 1.25.10+)
	$(GO) run ./tools/conformance/runner

.PHONY: verify-anchor
verify-anchor: ## Build the audit-anchor verifier CLI to bin/aether-verify-anchor.
	$(call require-tool,$(GO),install Go 1.25.10+)
	mkdir -p bin
	cd tools/aether-verify-anchor && $(GO) build -o ../../bin/aether-verify-anchor .

##@ Documentation

.PHONY: docs-serve
docs-serve: ## Serve the documentation site locally on :8000.
	$(call require-tool,$(PYTHON),install Python 3.11+)
	$(PYTHON) -m mkdocs serve

.PHONY: docs-build
docs-build: ## Build the static documentation site.
	$(call require-tool,$(PYTHON),install Python 3.11+)
	$(PYTHON) -m mkdocs build --strict

##@ Hygiene

.PHONY: fmt
fmt: ## Format Go source.
	$(GO) fmt ./...

.PHONY: clean
clean: ## Remove build artifacts.
	$(GO) clean ./...
	rm -rf bin/ dist/ site/

.PHONY: dco-check
dco-check: ## Verify all commits in the current branch have a DCO sign-off.
	@bad=$$(git log --no-merges --format='%H %s' origin/main..HEAD 2>/dev/null | \
	  while read sha _; do \
	    git log -1 --format=%B "$$sha" | grep -q '^Signed-off-by:' || echo "$$sha"; \
	  done); \
	if [ -n "$$bad" ]; then \
	  echo "commits missing DCO sign-off:"; \
	  echo "$$bad"; \
	  exit 1; \
	fi
	@echo "ok: all commits signed off"
