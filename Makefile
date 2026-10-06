# go-selfupdate-lib — the fleet's self-update library
# (no binary packaging targets).
#
# Prefer the user's Go toolchain install (go install ...), then PATH.
GOPATH_BIN    := $(shell go env GOPATH)/bin
GOBIN         := $(shell go env GOBIN)
GOLANGCI_LINT ?= $(GOPATH_BIN)/golangci-lint
GOVULNCHECK   ?= $(or $(wildcard $(GOBIN)/govulncheck),$(wildcard $(GOPATH_BIN)/govulncheck),$(shell command -v govulncheck 2>/dev/null))
GOTESTSUM     ?= $(or $(wildcard $(GOBIN)/gotestsum),$(wildcard $(GOPATH_BIN)/gotestsum),$(shell command -v gotestsum 2>/dev/null))
FLEET_LINT_CFG := .golangci.yml

.PHONY: all help test test-sum fmt vet lint tidy vuln apicheck fuzz pre-add-check

all: help

test: ## Runs all tests
	go test ./...

test-sum: ## Runs tests via gotestsum (opt-in; requires gotestsum on PATH/GOBIN)
	@if [ -z "$(GOTESTSUM)" ] || [ ! -x "$(GOTESTSUM)" ]; then \
		echo "gotestsum not found. Install: go install gotest.tools/gotestsum@latest"; \
		exit 1; \
	fi
	$(GOTESTSUM) --format testname -- ./...

fmt: ## Formats all Go source files
	go fmt ./...

vet: ## Runs go vet
	go vet ./...

# Lint every target the code builds for, with cgo off so a cross-target run
# never needs a C toolchain for that target
# (docs/decisions/0002-MADR-rehome-selfupdate-from-mcplib.md §5).
LINT_GOOS := linux darwin windows

lint: ## Runs golangci-lint with fleet config for linux, darwin and windows
	@if [ ! -x "$(GOLANGCI_LINT)" ]; then \
		echo "golangci-lint not found at $(GOLANGCI_LINT)"; \
		echo "Install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0"; \
		exit 1; \
	fi
	@status=0; for os in $(LINT_GOOS); do \
		echo "golangci-lint (GOOS=$$os)"; \
		CGO_ENABLED=0 GOOS=$$os $(GOLANGCI_LINT) run -c $(FLEET_LINT_CFG) ./... || { echo "golangci-lint failed for GOOS=$$os"; status=1; }; \
	done; exit $$status

tidy: ## Runs go mod tidy
	go mod tidy

vuln: ## Runs govulncheck (opt-in; requires govulncheck on PATH/GOBIN)
	@if [ -z "$(GOVULNCHECK)" ] || [ ! -x "$(GOVULNCHECK)" ]; then \
		echo "govulncheck not found. Install: go install golang.org/x/vuln/cmd/govulncheck@v1.8.0"; \
		exit 1; \
	fi
	$(GOVULNCHECK) ./...

# The exported API must stay compatible with the newest v1.* tag; pass
# BASE=<rev> to compare against another revision
# (docs/decisions/0004-PLAN-v1-1-0-core-api.md Step 12).
BASE ?=
apicheck: ## Fails on an incompatible exported API change (apidiff against the newest v1.* tag)
	@./scripts/check-api-compat.sh $(BASE)

# Fuzz each selfupdate fuzz target for FUZZTIME; go test -fuzz takes one
# target per run (docs/decisions/0004-PLAN-h2-fuzzing-and-manifest-differential.md).
# -m is the number of targets, so a lost one fails the run
# (docs/decisions/0004-PLAN-v1-4-0-command-surface.md Step 3).
FUZZTIME ?= 20s
# selfupdate/archive's three targets run after selfupdate's
# (docs/decisions/0012-PLAN-archive-assets-and-macos-codesign.md U3), then
# selfupdate/releasespec's one
# (docs/decisions/0013-PLAN-build-and-stage-release-workflow.md B1).
fuzz: ## Fuzzes every selfupdate, archive and releasespec fuzz target for FUZZTIME each (default 20s)
	@./scripts/go-fuzz.sh -t $(FUZZTIME) -m 5 ./selfupdate
	@./scripts/go-fuzz.sh -t $(FUZZTIME) -m 3 ./selfupdate/archive
	@./scripts/go-fuzz.sh -t $(FUZZTIME) -m 1 ./selfupdate/releasespec

# The pre-add rule (AGENTS.md). scripts/go-precheck.sh is the one
# implementation; the agent gate at `git commit` runs the same file.
# Pass FILES=... to check specific files instead of every tracked Go file.
FILES ?=
pre-add-check: ## Runs the pre-add checks (gofmt, golangci-lint, vet, test, govulncheck)
	@./scripts/go-precheck.sh $(FILES)

help: ## Displays this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
