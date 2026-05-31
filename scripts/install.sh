#!/usr/bin/env bash
set -euo pipefail

# diffcat installer
# Usage: git clone ... && cd diffcat && ./scripts/install.sh

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
RESET='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_NAME="diffcat"
GO_MIN_VERSION="1.21"

info()  { printf "${CYAN}[*]${RESET} %s\n" "$*"; }
ok()    { printf "${GREEN}[+]${RESET} %s\n" "$*"; }
warn()  { printf "${YELLOW}[!]${RESET} %s\n" "$*"; }
fail()  { printf "${RED}[x]${RESET} %s\n" "$*"; exit 1; }

# -------------------------------------------------------------------
# System checks
# -------------------------------------------------------------------

check_os() {
  case "$(uname -s)" in
    Darwin) OS="macos" ;;
    Linux)  OS="linux" ;;
    *)      fail "Unsupported OS: $(uname -s). macOS or Linux required." ;;
  esac
  ok "OS: $(uname -s) $(uname -m)"
}

check_git() {
  if ! command -v git &>/dev/null; then
    fail "git is not installed. Install it first:
    macOS:  xcode-select --install
    Linux:  sudo apt install git  (or your distro's equivalent)"
  fi
  ok "git: $(git --version | head -1)"
}

# Compare semver: returns 0 if $1 >= $2
version_gte() {
  [ "$(printf '%s\n' "$1" "$2" | sort -V | head -1)" = "$2" ]
}

check_go() {
  if ! command -v go &>/dev/null; then
    fail "go is not installed. Install it first:
    macOS:  brew install go
    Linux:  https://go.dev/doc/install"
  fi
  local ver
  ver="$(go version | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -1)"
  if ! version_gte "$ver" "$GO_MIN_VERSION"; then
    fail "go v${ver} found but v${GO_MIN_VERSION}+ required."
  fi
  ok "go: v${ver}"
}

# -------------------------------------------------------------------
# Build & install
# -------------------------------------------------------------------

build_binary() {
  info "Building diffcat..."
  cd "$REPO_ROOT"
  mkdir -p bin
  local version
  version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
  go build -ldflags "-X main.ldflagsVersion=${version#v}" -o bin/"$BIN_NAME" ./cmd/diffcat
  ok "Binary built: bin/$BIN_NAME (v${version#v})"
}

install_binary() {
  local bin_dir

  if [ -w "/usr/local/bin" ]; then
    bin_dir="/usr/local/bin"
  else
    bin_dir="$HOME/.local/bin"
    mkdir -p "$bin_dir"
  fi

  cp "bin/$BIN_NAME" "$bin_dir/$BIN_NAME"
  chmod +x "$bin_dir/$BIN_NAME"
  ok "Installed: $bin_dir/$BIN_NAME"

  if ! echo "$PATH" | tr ':' '\n' | grep -qx "$bin_dir"; then
    warn "$bin_dir is not on your PATH"
    printf "${YELLOW}  Add this to your shell profile:${RESET}\n"
    printf "${BOLD}    export PATH=\"\$HOME/.local/bin:\$PATH\"${RESET}\n"
    echo ""
  fi

  # Export for use in the closing message
  INSTALL_BIN_DIR="$bin_dir"
}

# -------------------------------------------------------------------
# Main
# -------------------------------------------------------------------

info "Starting installation..."
echo ""

check_os
check_git
check_go
echo ""

build_binary
install_binary

echo ""
printf "${GREEN}${BOLD}  Installation complete!${RESET}\n"
echo ""
printf "  ${BOLD}Run it:${RESET}\n"
printf "    ${CYAN}diffcat${RESET}                  # diff the current repo against master/main\n"
printf "    ${CYAN}diffcat --base develop${RESET}   # diff against a specific base ref\n"
printf "    ${CYAN}diffcat files${RESET}            # non-interactive changed-file list\n"
echo ""
printf "  ${DIM}Source: $REPO_ROOT${RESET}\n"
printf "  ${DIM}Binary: ${INSTALL_BIN_DIR:-/usr/local/bin}/$BIN_NAME${RESET}\n"
echo ""
