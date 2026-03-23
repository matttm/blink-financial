#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/install_docker_compose.sh

Installs Docker and Docker Compose using the platform-native path:
  - macOS: Homebrew + Docker Desktop
  - Linux: Docker Engine + Docker Compose plugin via apt
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

os="$(uname -s)"

case "$os" in
  Darwin)
    require_cmd brew

    if [[ -d "/Applications/Docker.app" ]]; then
      echo "Docker Desktop already appears to be installed at /Applications/Docker.app"
    else
      echo "Installing Docker Desktop with Homebrew..."
      brew install --cask docker
    fi

    echo "Verifying Docker CLI..."
    if command -v docker >/dev/null 2>&1; then
      docker --version
    else
      echo "Docker CLI not yet on PATH. Start Docker Desktop once to finish setup." >&2
    fi

    echo "Checking Compose support..."
    if docker compose version >/dev/null 2>&1; then
      docker compose version
    else
      echo "Compose plugin is not available yet. Start Docker Desktop and retry." >&2
    fi
    ;;
  Linux)
    require_cmd sudo
    require_cmd apt-get
    require_cmd install
    require_cmd curl

    echo "Installing Docker Engine and Docker Compose plugin on Linux..."
    sudo apt-get update
    sudo apt-get install -y ca-certificates curl
    sudo install -m 0755 -d /etc/apt/keyrings

    if [[ ! -f /etc/apt/keyrings/docker.asc ]]; then
      sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
      sudo chmod a+r /etc/apt/keyrings/docker.asc
    fi

    arch="$(dpkg --print-architecture)"
    codename="$(
      . /etc/os-release
      echo "${VERSION_CODENAME}"
    )"

    repo_line="deb [arch=${arch} signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu ${codename} stable"
    echo "${repo_line}" | sudo tee /etc/apt/sources.list.d/docker.list >/dev/null

    sudo apt-get update
    sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

    docker --version
    docker compose version
    ;;
  *)
    echo "Unsupported OS: ${os}" >&2
    exit 1
    ;;
esac
