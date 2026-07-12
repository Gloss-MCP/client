BINARY  := gloss
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

TAILWIND_VERSION := 4.3.2
TAILWIND_BIN      := .tailwindcli/tailwindcss
TAILWIND_CSS_IN   := internal/server/static/css/input.css
TAILWIND_CSS_OUT  := internal/server/static/css/app.css

GO_IMAGE := golang:1.25-alpine

# Fall back to Docker when local go is absent or pre-1.25
_go_ok := $(shell \
  v=$$(go version 2>/dev/null | grep -oE 'go[0-9]+\.[0-9]+' | sed 's/go//'); \
  if [ -z "$$v" ]; then echo no; exit 0; fi; \
  maj=$$(echo $$v | cut -d. -f1); min=$$(echo $$v | cut -d. -f2); \
  if [ "$$maj" -gt 1 ] || { [ "$$maj" -eq 1 ] && [ "$$min" -ge 25 ]; }; \
  then echo yes; else echo no; fi)

_DOCKER_RUN := docker run --rm \
  -v "$(CURDIR):/src" \
  -v "$(HOME)/go/pkg/mod:/go/pkg/mod" \
  -v "$(HOME)/.cache/go-build:/root/.cache/go-build" \
  -w /src $(GO_IMAGE)

ifeq ($(_go_ok),yes)
GO    := go
GOFMT := gofmt
else
GO    := $(_DOCKER_RUN) go
GOFMT := $(_DOCKER_RUN) gofmt
endif

.PHONY: build test lint e2e run clean assets

# assets compiles the vendored HTMX/Alpine.js + Tailwind CSS pipeline.
# The Tailwind standalone CLI is downloaded on first use and cached in
# .tailwindcli/ (gitignored). Network failures (offline dev, a sandboxed
# CI runner without GitHub access) are non-fatal: assets falls back to
# whatever internal/server/static/css/app.css is already committed,
# since that file is checked in precisely so `go build`/`go test` never
# require network access.
assets:
	@if [ ! -x "$(TAILWIND_BIN)" ]; then \
		echo "assets: fetching Tailwind CLI v$(TAILWIND_VERSION)..."; \
		mkdir -p .tailwindcli; \
		os=$$(uname -s); arch=$$(uname -m); \
		case "$$os" in \
			Linux) platform=linux ;; \
			Darwin) platform=macos ;; \
			*) platform= ;; \
		esac; \
		case "$$arch" in \
			x86_64|amd64) tarch=x64 ;; \
			arm64|aarch64) tarch=arm64 ;; \
			*) tarch= ;; \
		esac; \
		if [ -z "$$platform" ] || [ -z "$$tarch" ]; then \
			echo "assets: unsupported platform $$os/$$arch, keeping committed app.css"; \
		else \
			url="https://github.com/tailwindlabs/tailwindcss/releases/download/v$(TAILWIND_VERSION)/tailwindcss-$$platform-$$tarch"; \
			if curl -fsSL -o "$(TAILWIND_BIN).tmp" "$$url"; then \
				mv "$(TAILWIND_BIN).tmp" "$(TAILWIND_BIN)"; \
				chmod +x "$(TAILWIND_BIN)"; \
			else \
				echo "assets: could not download Tailwind CLI (no network?) -- keeping committed app.css"; \
				rm -f "$(TAILWIND_BIN).tmp"; \
			fi; \
		fi; \
	fi
	@if [ -x "$(TAILWIND_BIN)" ]; then \
		$(TAILWIND_BIN) -i $(TAILWIND_CSS_IN) -o $(TAILWIND_CSS_OUT) --minify; \
	fi

build: assets
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/gloss

test: assets
	$(GO) test ./...

lint: assets
	golangci-lint run
	@unformatted=$$($(GOFMT) -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: files need formatting:"; echo "$$unformatted"; exit 1; \
	fi

e2e: build
	cd e2e && npm ci && npx playwright test

run: build
	./$(BINARY) .

clean:
	rm -f $(BINARY)
	rm -rf .tailwindcli
