# Qorven — top-level Makefile
#
# Orchestrates the web UI static export + the Go backend build so a
# single binary comes out of `make`. The backend's own Makefile
# (backend/Makefile) still works standalone — this file wraps it for
# the full-release path.
#
# Targets:
#   make build                 — current-arch single binary with embedded UI → dist/qorven
#   make build-backend         — backend only (leaves embed/ alone)
#   make build-web             — Next.js static export → web/out/ + backend/internal/webui/dist/
#   make release               — cross-compile linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
#   make release-all           — same as release
#   make dev                   — backend on :4200 + Next.js dev server on :3000 in one shell
#   make clean                 — wipe dist/, web/out/, embed/
#   make verify                — run backend verify (vet + tests) and web typecheck
#
# Why top-level: users cloning the repo shouldn't have to know the
# two trees; `make` from the root does the right thing.

ROOT          := $(shell pwd)
WEB_DIR       := $(ROOT)/web
BACKEND_DIR   := $(ROOT)/backend
EMBED_DIR     := $(BACKEND_DIR)/internal/webui/dist
DIST_DIR      := $(ROOT)/dist
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GIT_COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_TIME    ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# Platforms we ship binaries for. Add/remove here and every release
# target picks up the change.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

HOST_ARCH := $(shell go env GOARCH 2>/dev/null || uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
HOST_OS   := $(shell go env GOOS   2>/dev/null || uname -s | tr '[:upper:]' '[:lower:]')

# Use the arm64-native Go toolchain when available (avoids QEMU overhead on
# ARM hosts that also have an amd64 Go install on $PATH).
GO_CMD    := $(shell if [ -x /usr/local/go-arm64/bin/go ]; then echo /usr/local/go-arm64/bin/go; else echo go; fi)

.PHONY: help all build build-backend build-web build-sidecar release release-all dev clean verify \
        typecheck-web check-sse-names font-ban-web check-plan-authz kill-builds

help:  ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

all: build  ## Default: build the embedded binary

build: build-web build-backend  ## Build the embedded single-binary (current arch)
	@size=$$(du -h $(DIST_DIR)/qorven | awk '{print $$1}'); \
	echo "==> $(DIST_DIR)/qorven built ($$size, $$(uname -m), $(VERSION))"

build-web:  ## Compile the Next.js static export into the Go embed dir
	@command -v pnpm >/dev/null 2>&1 || { echo "pnpm required: npm install -g pnpm"; exit 1; }
	cd $(WEB_DIR) && pnpm install --frozen-lockfile
	cd $(WEB_DIR) && QORVEN_STATIC=1 NEXT_PUBLIC_API_URL= NEXT_PUBLIC_API_TOKEN= pnpm build
	rm -rf $(EMBED_DIR)
	mkdir -p $(EMBED_DIR)
	cp -r $(WEB_DIR)/out/. $(EMBED_DIR)/
	printf '*\n!.gitignore\n!.embedded\n' > $(EMBED_DIR)/.gitignore
	printf 'Placeholder so go:embed has a file. Release builds overwrite this directory.\n' > $(EMBED_DIR)/.embedded

build-backend:  ## Build the Go binary for the current arch → dist/qorven
	@if [ -n "$(GOARCH)" ] && [ "$(GOARCH)" != "$(HOST_ARCH)" ]; then \
		echo ""; \
		echo "  WARNING: GOARCH=$(GOARCH) differs from host $(HOST_ARCH)."; \
		echo "  Cross-compilation on this ARM machine takes 3–5 min and stacks badly."; \
		echo "  Use 'make release-linux-$(GOARCH)' for a deliberate cross-compile."; \
		echo "  Unset GOARCH to build natively (fast)."; \
		echo ""; \
	fi
	@mkdir -p $(DIST_DIR)
	cd $(BACKEND_DIR) && CGO_ENABLED=0 $(GO_CMD) build \
		-trimpath \
		-ldflags "-s -w \
			-X 'github.com/qorvenai/qorven/cmd.Version=$(VERSION)' \
			-X 'github.com/qorvenai/qorven/cmd.Commit=$(GIT_COMMIT)' \
			-X 'github.com/qorvenai/qorven/cmd.BuildTime=$(BUILD_TIME)'" \
		-o $(DIST_DIR)/qorven .

build-sidecar:  ## Build the WhatsApp sidecar
	cd sidecar/whatsapp && npm ci && npm run build
	@echo "✓ WhatsApp sidecar built at sidecar/whatsapp/dist/index.js"

kill-builds:  ## Kill any stale background go build / go run processes
	@echo "==> Killing stale go build/run processes…"
	@pkill -f "go build" 2>/dev/null && echo "   killed go build" || echo "   no go build running"
	@pkill -f "go run"   2>/dev/null && echo "   killed go run"   || echo "   no go run running"
	@pkill -f "qemu-x86_64" 2>/dev/null && echo "   killed qemu-x86_64" || echo "   no qemu running"
	@echo "done"

# release-<goos>-<goarch> — cross-compile one platform. Used by CI and
# by `make release` below. Each target writes dist/qorven-<os>-<arch>
# with a sha256 sidecar so the release pipeline can upload + verify.
#
# Windows gets the .exe suffix automatically.
define RELEASE_template
.PHONY: release-$(1)-$(2)
release-$(1)-$(2):
	@mkdir -p $(DIST_DIR)
	@OUT=$(DIST_DIR)/qorven-$(1)-$(2); \
	if [ "$(1)" = "windows" ]; then OUT=$$$$OUT.exe; fi; \
	echo "==> building $$$$OUT"; \
	cd $(BACKEND_DIR) && CGO_ENABLED=0 GOOS=$(1) GOARCH=$(2) $(GO_CMD) build \
		-trimpath -ldflags "-s -w \
			-X 'github.com/qorvenai/qorven/cmd.Version=$(VERSION)' \
			-X 'github.com/qorvenai/qorven/cmd.Commit=$(GIT_COMMIT)' \
			-X 'github.com/qorvenai/qorven/cmd.BuildTime=$(BUILD_TIME)'" \
		-o $$$$OUT . ; \
	cd $(DIST_DIR) && sha256sum $$$$(basename $$$$OUT) > $$$$(basename $$$$OUT).sha256 ; \
	cat $(DIST_DIR)/$$$$(basename $$$$OUT).sha256
endef

$(foreach pair,$(PLATFORMS),$(eval $(call RELEASE_template,$(word 1,$(subst /, ,$(pair))),$(word 2,$(subst /, ,$(pair))))))

release: build-web  ## Cross-compile every platform with the embedded UI
	@$(MAKE) release-all-platforms

release-all-platforms:
	$(foreach pair,$(PLATFORMS),$(MAKE) release-$(word 1,$(subst /, ,$(pair)))-$(word 2,$(subst /, ,$(pair))) &&) true
	@echo "==> release binaries in $(DIST_DIR)/"
	@ls -lh $(DIST_DIR)/

release-all: release  ## Alias

dev:  ## Run backend + Next.js dev servers
	@./scripts/dev-up.sh

verify: typecheck-web lint-web font-ban-web check-sse-names check-plan-authz  ## Run web typecheck + lint + font-ban + backend verify
	$(MAKE) -C $(BACKEND_DIR) verify

check-plan-authz:  ## Fail if plan handlers use old string-returning planAuthorize (FU-017)
	@hits=$$(grep -rn "planAuthorize\b" \
	  $(BACKEND_DIR)/internal/gateway/ \
	  | grep -v "_test.go" \
	); \
	if [ -n "$$hits" ]; then \
	  echo "ERROR: planAuthorize() still present — use authorizeForPlan + writePlanAuthzError (FU-017):"; \
	  echo "$$hits"; \
	  exit 1; \
	fi; \
	echo "==> check-plan-authz: OK (no planAuthorize references in gateway handlers)"

font-ban-web:  ## Ban thin/light font weights and inline fontFamily in web/ source files
	@hits=$$(grep -rn \
	  --include="*.tsx" --include="*.ts" --include="*.css" \
	  -e 'font-thin' -e 'font-extralight' -e 'font-light' \
	  -e "fontFamily[[:space:]]*:" \
	  $(WEB_DIR)/app $(WEB_DIR)/components $(WEB_DIR)/lib \
	  | grep -v "node_modules" \
	  | grep -v "//.*ok" \
	); \
	if [ -n "$$hits" ]; then \
	  echo "ERROR: banned font usage found in web/:"; \
	  echo "$$hits"; \
	  echo "Use font-normal (400) or font-medium (500) or font-semibold (600) instead."; \
	  echo "For inline fontFamily overrides add '// ok' at end of line if intentional."; \
	  exit 1; \
	fi; \
	echo "==> font-ban-web: OK (no thin/light weights or inline fontFamily found)"

check-sse-names:  ## Verify all broadcast/sendSSE names are in legacyAliases (FU-004)
	@bash $(ROOT)/scripts/check-sse-names.sh

typecheck-web:
	cd $(WEB_DIR) && pnpm install --frozen-lockfile && pnpm exec tsc --noEmit

lint-web:  ## ESLint report (informational — errors below are pre-existing any/unused-vars)
	cd $(WEB_DIR) && pnpm exec eslint app components lib store || true

clean:  ## Wipe dist/, web/out/, embed/
	rm -rf $(DIST_DIR) $(WEB_DIR)/out $(WEB_DIR)/.next
	rm -rf $(EMBED_DIR)
	mkdir -p $(EMBED_DIR)
	printf '*\n!.gitignore\n!.embedded\n' > $(EMBED_DIR)/.gitignore
	printf 'Placeholder so go:embed has a file. Release builds overwrite this directory.\n' > $(EMBED_DIR)/.embedded
