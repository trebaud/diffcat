#!/usr/bin/env bash
set -euo pipefail

# sashi deploy script
# Warms the Go module proxy for the current latest tag so
# `go install github.com/trebaud/sashi/cmd/sashi@latest` picks it up.

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

info() { printf "${CYAN}[*]${RESET} %s\n" "$*"; }
ok()   { printf "${GREEN}[+]${RESET} %s\n" "$*"; }
warn() { printf "${YELLOW}[!]${RESET} %s\n" "$*"; }
fail() { printf "${RED}[x]${RESET} %s\n" "$*"; exit 1; }

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

MODULE_PATH="$(go list -m 2>/dev/null || echo github.com/trebaud/sashi)"

[ -d .git ] || fail "Not a git repository."

git fetch --tags --quiet origin

TAG="$(git tag -l 'v*.*.*' | sort -V | tail -1)"
[ -z "$TAG" ] && fail "No semver tags found."
ok "Current version: $TAG"

# -------------------------------------------------------------------
# Check / warm the proxy (one request: hit triggers a fetch if not cached)
# -------------------------------------------------------------------

PROXY_URL="https://proxy.golang.org/${MODULE_PATH}/@v/${TAG}.info"
info "Hitting proxy.golang.org for ${MODULE_PATH}@${TAG}..."
if curl -fsSL "$PROXY_URL" >/dev/null 2>&1; then
  ok "Version is available on the proxy"
else
  warn "Proxy returned an error — the tag may not be pushed yet or propagation is still in progress."
  exit 1
fi

echo ""
printf "${GREEN}${BOLD}  Deployed ${TAG}${RESET}\n\n"
printf "  Install with:\n"
printf "    ${CYAN}go install ${MODULE_PATH}/cmd/sashi@latest${RESET}\n"
printf "    ${CYAN}go install ${MODULE_PATH}/cmd/sashi@${TAG}${RESET}\n"
echo ""
