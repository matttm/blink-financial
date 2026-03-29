#!/usr/bin/env bash

set -euo pipefail

DEFAULT_MOUNTPOINT="/Volumes/blink-ramdisk"
REMOVE_DIR=false

usage() {
  cat <<'EOF'
Usage:
  scripts/cleanup_ramdisk.sh [mountpoint] [--remove-dir]

Examples:
  scripts/cleanup_ramdisk.sh
  scripts/cleanup_ramdisk.sh /Volumes/blink-ledger
  scripts/cleanup_ramdisk.sh /Volumes/blink-ledger --remove-dir

Notes:
  - Unmounts a RAM-backed filesystem if one is mounted at the target path.
  - Does not remove the mountpoint directory unless --remove-dir is provided.
EOF
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
}

mountpoint_path="$DEFAULT_MOUNTPOINT"

for arg in "$@"; do
  case "$arg" in
    -h|--help)
      usage
      exit 0
      ;;
    --remove-dir)
      REMOVE_DIR=true
      ;;
    *)
      mountpoint_path="$arg"
      ;;
  esac
done

remove_mountpoint_dir() {
  if [[ "$REMOVE_DIR" != "true" ]]; then
    return 0
  fi

  if [[ -d "$mountpoint_path" ]]; then
    if rmdir "$mountpoint_path" 2>/dev/null; then
      echo "Removed empty mountpoint directory: $mountpoint_path"
    else
      echo "Left mountpoint directory in place because it is not empty: $mountpoint_path"
    fi
  fi
}

os="$(uname -s)"

case "$os" in
  Darwin)
    require_cmd diskutil
    require_cmd hdiutil

    if ! mount | grep -F "on ${mountpoint_path} " >/dev/null 2>&1; then
      echo "No mounted filesystem found at ${mountpoint_path}."
      remove_mountpoint_dir
      exit 0
    fi

    device="$(mount | awk -v target="$mountpoint_path" '$0 ~ " on " target " " {print $1; exit}')"

    echo "Unmounting RAM disk at ${mountpoint_path}..."
    diskutil unmount "${mountpoint_path}" >/dev/null

    if [[ -n "$device" ]]; then
      hdiutil detach "${device}" >/dev/null || true
    fi

    echo "RAM disk cleaned up: ${mountpoint_path}"
    remove_mountpoint_dir
    ;;
  Linux)
    require_cmd mountpoint
    require_cmd umount

    if ! mountpoint -q "${mountpoint_path}"; then
      echo "No mounted filesystem found at ${mountpoint_path}."
      remove_mountpoint_dir
      exit 0
    fi

    echo "Unmounting RAM disk at ${mountpoint_path}..."
    umount "${mountpoint_path}"

    echo "RAM disk cleaned up: ${mountpoint_path}"
    remove_mountpoint_dir
    ;;
  *)
    echo "Unsupported OS: ${os}" >&2
    exit 1
    ;;
esac
