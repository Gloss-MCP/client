#!/bin/bash
# Verifies the tooling Gloss needs (Go, golangci-lint, Tailwind CLI, e2e deps)
# and installs whatever is missing. Remote (cloud) sessions only.
set -euo pipefail

if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

cd "${CLAUDE_PROJECT_DIR:-$(pwd)}"

GOLANGCI_LINT_VERSION="v2.5.0" # keep in sync with .github/workflows/ci.yml

log() { echo "[session-start] $*"; }

# --- Go (go.mod requires 1.25+; the go command fetches the toolchain itself) ---
if ! command -v go >/dev/null 2>&1; then
  log "go not found; installing"
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq && apt-get install -y -qq golang-go
  else
    log "ERROR: go is missing and no supported package manager found"; exit 1
  fi
fi
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
go version >/dev/null # triggers toolchain download per go.mod if needed
log "go: $(go version)"
go mod download

GOBIN="$(go env GOPATH)/bin"
case ":$PATH:" in *":$GOBIN:"*) ;; *) export PATH="$GOBIN:$PATH" ;; esac
[ -n "${CLAUDE_ENV_FILE:-}" ] && echo "export PATH=\"$GOBIN:\$PATH\"" >> "$CLAUDE_ENV_FILE"

# --- golangci-lint (pinned to CI's version) ---
if ! golangci-lint version 2>&1 | grep -q "version ${GOLANGCI_LINT_VERSION#v}"; then
  log "installing golangci-lint ${GOLANGCI_LINT_VERSION}"
  go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"
fi
log "golangci-lint: $(golangci-lint version 2>&1 | head -1)"

# --- Tailwind standalone CLI (via the Makefile target; non-fatal offline) ---
make assets

# --- Playwright e2e deps (Chromium is pre-installed in cloud sandboxes) ---
if [ -f e2e/package.json ] && command -v npm >/dev/null 2>&1; then
  if [ ! -d e2e/node_modules ]; then
    log "installing e2e npm dependencies"
    (cd e2e && PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 npm install --no-audit --no-fund)
  fi
  if [ -x /opt/pw-browsers/chromium ] && [ -n "${CLAUDE_ENV_FILE:-}" ]; then
    echo 'export PW_EXECUTABLE_PATH="/opt/pw-browsers/chromium"' >> "$CLAUDE_ENV_FILE"
  fi
fi

log "done"
