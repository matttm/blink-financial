#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/install_k6.sh

Installs k6 using the platform-native path:
  - macOS: Homebrew
  - Linux: Debian/Ubuntu apt repository
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
}

if command -v k6 >/dev/null 2>&1; then
  echo "k6 is already installed:"
  k6 version
  exit 0
fi

os="$(uname -s)"

case "$os" in
  Darwin)
    require_cmd brew

    echo "Installing k6 with Homebrew..."
    brew install k6
    ;;
  Linux)
    require_cmd sudo
    require_cmd apt-get
    require_cmd curl
    require_cmd gpg

    echo "Installing k6 on Debian/Ubuntu..."
    sudo apt-get update
    sudo apt-get install -y ca-certificates curl gnupg
    sudo install -m 0755 -d /etc/apt/keyrings

    if [[ ! -f /etc/apt/keyrings/k6.gpg ]]; then
      curl -fsSL https://dl.k6.io/key.gpg | sudo gpg --dearmor -o /etc/apt/keyrings/k6.gpg
      sudo chmod a+r /etc/apt/keyrings/k6.gpg
    fi

    echo "deb [signed-by=/etc/apt/keyrings/k6.gpg] https://dl.k6.io/deb stable main" |
      sudo tee /etc/apt/sources.list.d/k6.list >/dev/null

    sudo apt-get update
    sudo apt-get install -y k6
    ;;
  *)
    echo "Unsupported OS: ${os}" >&2
    exit 1
    ;;
esac

echo "Verifying k6 installation..."
k6 version
