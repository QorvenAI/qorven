# Qorven backend build + verify targets.
#
# Phase 3 gate requirement: `make build` and `make verify` are the
# canonical entry points for CI. Every pre-release build MUST run
# `make verify` and `make build` and attach the resulting binary.
# Hand-run commands drift; these targets do not.

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     := $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    = -s -w \
             -X github.com/qorvenai/qorven/cmd.Version=$(VERSION) \
             -X github.com/qorvenai/qorven/cmd.Commit=$(COMMIT) \
             -X github.com/qorvenai/qorven/cmd.BuildTime=$(BUILD_TIME)
BINARY     = qorven
DIST_DIR   = dist

# Tests that require a running Postgres are skipped when QORVEN_TEST_DSN
# is unreachable. Set this env var in CI to exercise them.
export CGO_ENABLED ?= 0

.PHONY: all build build-release build-release-arch build-release-all run clean \
        dev test test-short test-integration wasm-testdata vet lint verify ui migrate-up migrate-down docker \
        docker-down version tidy help

all: verify build

## build        — development build of the qorven binary.
build:
	CGO_ENABLED=$(CGO_ENABLED) go build -ldflags="$(LDFLAGS)" -o $(BINARY) .

## build-release — reproducible, stripped production binary in dist/.
##                  Honors GOOS/GOARCH when set; defaults to host triple.
build-release:
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY) .
	@echo "built $(DIST_DIR)/$(BINARY)"
	@file $(DIST_DIR)/$(BINARY) | head -1
	@ls -la $(DIST_DIR)/$(BINARY)
	@cd $(DIST_DIR) && sha256sum $(BINARY) > $(BINARY).sha256
	@cat $(DIST_DIR)/$(BINARY).sha256

## build-release-arch — per-arch release; produces
##                      dist/qorven-$(GOOS)-$(GOARCH) + sha256. Used by the
##                      CI matrix so each (os,arch) pair lands as a
##                      distinct artifact. GOOS/GOARCH must be set by the
##                      caller (CI passes them; operators invoke manually).
build-release-arch:
	@if [ -z "$(GOOS)" ] || [ -z "$(GOARCH)" ]; then \
		echo "ERROR: GOOS and GOARCH must be set for build-release-arch"; \
		echo "       e.g. GOOS=linux GOARCH=arm64 make build-release-arch"; \
		exit 1; \
	fi
	@mkdir -p $(DIST_DIR)
	@OUT=$(DIST_DIR)/$(BINARY)-$(GOOS)-$(GOARCH); \
		CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags="$(LDFLAGS)" -o $$OUT . && \
		echo "built $$OUT" && \
		file $$OUT | head -1 && \
		ls -la $$OUT && \
		cd $(DIST_DIR) && sha256sum $(BINARY)-$(GOOS)-$(GOARCH) > $(BINARY)-$(GOOS)-$(GOARCH).sha256 && \
		cat $(BINARY)-$(GOOS)-$(GOARCH).sha256

## build-release-all — convenience: produces BOTH linux/amd64 and linux/arm64
##                     release binaries in dist/. Used for local release
##                     dry-runs; CI invokes build-release-arch directly
##                     via matrix.
##                     NOTE: on ARM dev machines this cross-compiles amd64
##                     which takes ~4 min each. Run sequentially, never
##                     in parallel with other builds.
build-release-all:
	@echo "==> build-release-all: sequentially cross-compiling linux/amd64 then linux/arm64"
	@echo "    This takes ~4 min per arch on ARM. Do not run other builds in parallel."
	$(MAKE) GOOS=linux GOARCH=amd64 build-release-arch
	$(MAKE) GOOS=linux GOARCH=arm64 build-release-arch

## verify       — the CI gate: format check, vet, staticcheck, authz-check, and full test suite.
##                Every pre-release run MUST pass this.
verify: tidy-check vet staticcheck authz-check test

## authz-check  — FU-017 guard: reject inline role checks that bypass authorize.go.
##                The three patterns below must not appear in gateway handler files
##                outside of authorize.go and the two thin adapter shims
##                (planAuthorize in plan_handlers.go, protocolOwnerCheck in protocol.go).
##                All real authorization logic lives in authorize.go.
authz-check:
	@echo "authz-check: scanning for bypassed authorization..."
	@! grep -rn --include="*.go" \
		-e 'role.*==.*"admin"' \
		-e 'IsServiceAccount' \
		-e 'serviceAccounts\.Lookup' \
		internal/gateway/ \
		| grep -v "authorize\.go" \
		| grep -v "protocol\.go" \
		| grep -v "plan_handlers\.go" \
		| grep -v "_test\.go" \
		| grep -q . \
	|| { echo "ERROR: raw authorization logic found outside authorize.go — consolidate via gw.authorize()"; exit 1; }
	@echo "authz-check: OK"

## vet          — go vet all packages.
vet:
	CGO_ENABLED=$(CGO_ENABLED) go vet ./...

## staticcheck  — deep static analysis: unused values, wrong API usage, simplifications.
##                Excludes style noise (ST1000/ST1022/ST1003/ST1020/ST1021 doc comments,
##                U1000 dead-code, SA1019 deprecated) — see staticcheck.conf.
##                These are tracked separately and don't block CI.
staticcheck:
	@if ! command -v staticcheck >/dev/null 2>&1; then \
		echo "staticcheck not installed — run: go install honnef.co/go/tools/cmd/staticcheck@latest"; \
		exit 1; \
	fi
	@issues=$$(staticcheck ./... 2>&1 \
		| grep -v "U1000\|SA1019\|ST1000\|ST1022\|ST1003\|ST1020\|ST1021" \
		| grep -v "_test\.go" || true); \
	if [ -n "$$issues" ]; then \
		echo "$$issues"; \
		exit 1; \
	fi
	@echo "staticcheck: OK"

## errcheck     — verify all error return values are handled.
##                Exclusions for fmt.*, Close, os.Remove etc in .errcheck_excludes.
errcheck:
	@if command -v errcheck >/dev/null 2>&1; then \
		errcheck -exclude .errcheck_excludes ./...; \
	else \
		echo "errcheck not installed — run: go install github.com/kisielk/errcheck@latest"; \
		exit 1; \
	fi

## test         — full test suite (skips DB tests without QORVEN_TEST_DSN).
test:
	CGO_ENABLED=$(CGO_ENABLED) go test ./... -count=1 -timeout 5m

## test-short   — fast subset: skip DB + slow tests (-short).
test-short:
	CGO_ENABLED=$(CGO_ENABLED) go test ./... -count=1 -short -timeout 2m

## test-integration — opt-in live-provider tests (OpenAI / DeepSeek / Gemini).
##                   Requires provider API keys set as environment variables.
##                   NOT part of the hermetic `verify` gate — contributors
##                   without credits must still see a green CI.
test-integration:
	CGO_ENABLED=$(CGO_ENABLED) go test -tags=integration ./internal/providers/... -count=1 -timeout 10m

## wasm-testdata — rebuild sample Wasm plugin(s) used by the host test
##                  suite. Requires Go 1.21+ for native GOOS=wasip1 target.
##                  The compiled .wasm is checked in so CI does not need
##                  to cross-compile; run this when you edit a testdata
##                  .go source.
wasm-testdata:
	GOOS=wasip1 GOARCH=wasm go build \
		-o internal/plugins/wasm/testdata/echo_plugin.wasm \
		internal/plugins/wasm/testdata/echo_plugin.go
	@ls -la internal/plugins/wasm/testdata/echo_plugin.wasm

## tidy-check   — fails if go.mod / go.sum are out of date.
tidy-check:
	@cp go.mod go.mod.tidy-check && cp go.sum go.sum.tidy-check
	@trap 'mv go.mod.tidy-check go.mod; mv go.sum.tidy-check go.sum' EXIT; \
		go mod tidy; \
		if ! diff -q go.mod go.mod.tidy-check >/dev/null; then \
			echo "ERROR: go.mod is not tidy — run 'go mod tidy' and commit"; \
			exit 1; \
		fi; \
		if ! diff -q go.sum go.sum.tidy-check >/dev/null; then \
			echo "ERROR: go.sum is not tidy — run 'go mod tidy' and commit"; \
			exit 1; \
		fi

## tidy         — run go mod tidy.
tidy:
	go mod tidy

## lint         — golangci-lint (optional; CI can skip if not installed).
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed — skipping"; \
	fi

## run          — build + start the gateway.
run: build
	./$(BINARY) start

## dev          — run directly without producing a binary.
dev:
	go run -ldflags="$(LDFLAGS)" . start

## dev-watch    — hot-reload + auto-restart on crash via `air`.
## Install once:  go install github.com/air-verse/air@latest
## Reads backend/.air.toml. Port probe in the gateway means a stale
## process from a previous run won't block the reload.
dev-watch:
	@command -v air >/dev/null 2>&1 || { \
		echo "air not installed. Run: go install github.com/air-verse/air@latest"; \
		exit 1; \
	}
	air -c .air.toml

## clean        — remove local binaries.
clean:
	rm -f $(BINARY)
	rm -rf $(DIST_DIR)

## ui           — build the Next.js frontend.
ui:
	cd ../web && pnpm install --frozen-lockfile && pnpm build

## migrate-up   — apply database migrations.
migrate-up: build
	./$(BINARY) migrate up

## migrate-down — roll back one migration.
migrate-down: build
	./$(BINARY) migrate down

## docker       — bring up the docker-compose stack.
docker:
	docker compose up -d

## docker-down  — tear down the docker-compose stack.
docker-down:
	docker compose down

## version      — print the computed version string.
version:
	@echo $(VERSION)

## help         — list targets.
help:
	@grep -E '^##' Makefile | sed 's/## //'
