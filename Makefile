BINARY  := gloss
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short=7 HEAD 2>/dev/null)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

TAILWIND_VERSION := 4.3.2
TAILWIND_BIN      := .tailwindcli/tailwindcss
TAILWIND_CSS_IN   := internal/server/static/css/input.css
TAILWIND_CSS_OUT  := internal/server/static/css/app.css

GORELEASER_VERSION := 2.12.0
GORELEASER_BIN     := .goreleaser-bin/goreleaser

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

.PHONY: build test lint e2e run clean assets site goreleaser release-check release-snapshot release release-notes release-formula

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

# site compiles the static docs site's CSS (site/css/site.css, gitignored)
# with the same standalone Tailwind CLI. Unlike assets, it fails when the
# CLI is unavailable: there is no committed fallback to serve.
site: assets
	@test -x "$(TAILWIND_BIN)" || { echo "site: Tailwind CLI unavailable"; exit 1; }
	$(TAILWIND_BIN) -i site/css/input.css -o site/css/site.css --minify

# goreleaser fetches the pinned GoReleaser release into .goreleaser-bin/
# (gitignored), verifying the archive against a sha256 pinned here. Unlike
# assets this fails when the download does: there is no fallback binary.
goreleaser:
	@if [ ! -x "$(GORELEASER_BIN)" ]; then \
		os=$$(uname -s); arch=$$(uname -m); \
		case "$$os/$$arch" in \
			Linux/x86_64|Linux/amd64) asset=Linux_x86_64; \
				sum=0c8915d91a491b1349fe4288eb8541d3876c5187be7f7e614bfc3fe442cf06d1 ;; \
			Linux/aarch64|Linux/arm64) asset=Linux_arm64; \
				sum=d36fd20b66431dba7ee4c5ffc6f4d1b37c876d694d65ced1ff4911c143d8e66d ;; \
			Darwin/x86_64) asset=Darwin_x86_64; \
				sum=cfdee082f2c3e601561a019b562998ba1bd98f8ca5ee9e4e3ada40138ef2abd1 ;; \
			Darwin/arm64) asset=Darwin_arm64; \
				sum=dc483d740634eb41cf5647473fb9055b18bc60fb14192b9bb3ad5c1be4486a16 ;; \
			*) echo "goreleaser: unsupported platform $$os/$$arch"; exit 1 ;; \
		esac; \
		echo "goreleaser: fetching v$(GORELEASER_VERSION) ($$asset)..."; \
		mkdir -p .goreleaser-bin; \
		url="https://github.com/goreleaser/goreleaser/releases/download/v$(GORELEASER_VERSION)/goreleaser_$$asset.tar.gz"; \
		curl -fsSL -o .goreleaser-bin/goreleaser.tar.gz "$$url" \
			|| { echo "goreleaser: download failed (no network?)"; rm -rf .goreleaser-bin; exit 1; }; \
		if command -v sha256sum >/dev/null 2>&1; then got=$$(sha256sum .goreleaser-bin/goreleaser.tar.gz | cut -d' ' -f1); \
		else got=$$(shasum -a 256 .goreleaser-bin/goreleaser.tar.gz | cut -d' ' -f1); fi; \
		[ "$$got" = "$$sum" ] || { echo "goreleaser: checksum mismatch for $$asset"; rm -rf .goreleaser-bin; exit 1; }; \
		tar -xzf .goreleaser-bin/goreleaser.tar.gz -C .goreleaser-bin goreleaser; \
		rm -f .goreleaser-bin/goreleaser.tar.gz; \
	fi

# release-check validates .goreleaser.yaml.
release-check: goreleaser
	$(GORELEASER_BIN) check

# release-snapshot cross-compiles every release target and builds the
# archives and checksums.txt into dist/ without tagging or publishing.
release-snapshot: goreleaser
	$(GORELEASER_BIN) release --snapshot --clean --skip=publish

# release-notes prints the CHANGELOG.md section for VERSION; it fails when
# there is none, so a tag cannot ship without notes.
release-notes:
	@test -n "$(VERSION_TAG)" || { echo "release-notes: set VERSION_TAG, e.g. VERSION_TAG=v0.1.0"; exit 1; }
	@sh scripts/release-notes.sh "$(VERSION_TAG)"

# release-formula renders the Homebrew formula for VERSION_TAG from a
# published release's checksums.txt (SUMS).
release-formula:
	@test -n "$(VERSION_TAG)" && test -n "$(SUMS)" || { echo "release-formula: set VERSION_TAG and SUMS"; exit 1; }
	@sh scripts/render-formula.sh "$(VERSION_TAG)" "$(SUMS)"

# release builds and uploads a DRAFT GitHub release for the checked-out tag
# (CI only: needs GITHUB_TOKEN). NOTES is the release-notes file.
release: goreleaser
	@test -n "$(NOTES)" || { echo "release: set NOTES to a release-notes file"; exit 1; }
	$(GORELEASER_BIN) release --clean --release-notes="$(NOTES)"

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
	rm -rf .tailwindcli .goreleaser-bin dist
