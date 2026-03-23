#!/usr/bin/env bash

set -euo pipefail

DEFAULT_SIZE_MB=1024
DEFAULT_MOUNTPOINT="/Volumes/blink-ramdisk"

usage() {
  cat <<'EOF'
Usage:
  scripts/setup_ramdisk.sh [size_mb] [mountpoint]

Examples:
  scripts/setup_ramdisk.sh
  scripts/setup_ramdisk.sh 2048 /Volumes/blink-ledger
  scripts/setup_ramdisk.sh 1024 /mnt/blink-ramdisk

Notes:
  - macOS: creates and mounts an HFS+ RAM disk.
  - Linux: mounts a tmpfs RAM disk.
EOF
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
}

size_mb="${1:-$DEFAULT_SIZE_MB}"
mountpoint="${2:-$DEFAULT_MOUNTPOINT}"

if [[ "${size_mb}" == "-h" || "${size_mb}" == "--help" ]]; then
  usage
  exit 0
fi

if ! [[ "$size_mb" =~ ^[0-9]+$ ]] || [[ "$size_mb" -le 0 ]]; then
  echo "Size must be a positive integer in MB. Got: $size_mb" >&2
  exit 1
fi

os="$(uname -s)"

case "$os" in
  Darwin)
    require_cmd hdiutil
    require_cmd diskutil

    sectors=$((size_mb * 2048))

    if mount | grep -F "on ${mountpoint} " >/dev/null 2>&1; then
      echo "A volume is already mounted at ${mountpoint}."
      exit 0
    fi

    echo "Creating ${size_mb}MB RAM disk on macOS at ${mountpoint}..."
    device="$(hdiutil attach -nomount "ram://${sectors}")"
    diskutil erasevolume HFS+ "blink-ramdisk" "${device}" >/dev/null

    actual_mountpoint="/Volumes/blink-ramdisk"
    if [[ "${actual_mountpoint}" != "${mountpoint}" ]]; then
      mkdir -p "${mountpoint}"
      diskutil unmount "${actual_mountpoint}" >/dev/null
      mount -t hfs "${device}" "${mountpoint}"
      actual_mountpoint="${mountpoint}"
    fi

    echo "RAM disk ready: ${actual_mountpoint}"
    df -h "${actual_mountpoint}"
    ;;
  Linux)
    require_cmd mount
    require_cmd df

    mkdir -p "${mountpoint}"

    if mountpoint -q "${mountpoint}"; then
      echo "A filesystem is already mounted at ${mountpoint}."
      exit 0
    fi

    echo "Creating ${size_mb}MB tmpfs RAM disk on Linux at ${mountpoint}..."
    mount -t tmpfs -o "size=${size_mb}m" tmpfs "${mountpoint}"

    echo "RAM disk ready: ${mountpoint}"
    df -h "${mountpoint}"
    ;;
  *)
    echo "Unsupported OS: ${os}" >&2
    exit 1
    ;;
esac
