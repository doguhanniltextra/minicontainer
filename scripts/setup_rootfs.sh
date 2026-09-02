#!/usr/bin/env bash
set -euo pipefail

# Directory locations
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
ROOTFS_DIR="${PROJECT_ROOT}/assets/rootfs"

# Alpine mini rootfs release
ALPINE_VERSION="3.20.0"
ARCH="x86_64"
ROOTFS_URL="https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/${ARCH}/alpine-minirootfs-${ALPINE_VERSION}-${ARCH}.tar.gz"
TEMP_TAR="/tmp/alpine-minirootfs-${ALPINE_VERSION}-${ARCH}.tar.gz"

if [ -f "${ROOTFS_DIR}/bin/sh" ]; then
    echo "Alpine rootfs already exists at ${ROOTFS_DIR}. Skipping download."
    exit 0
fi

echo "Creating rootfs directory: ${ROOTFS_DIR}"
mkdir -p "${ROOTFS_DIR}"

echo "Downloading Alpine Linux mini rootfs (${ALPINE_VERSION})..."
if command -v curl >/dev/null 2>&1; then
    curl -fsSL "${ROOTFS_URL}" -o "${TEMP_TAR}"
elif command -v wget >/dev/null 2>&1; then
    wget -qO "${TEMP_TAR}" "${ROOTFS_URL}"
else
    echo "Error: curl or wget is required to download rootfs." >&2
    exit 1
fi

echo "Extracting rootfs to ${ROOTFS_DIR}..."
tar -xzf "${TEMP_TAR}" -C "${ROOTFS_DIR}"
rm -f "${TEMP_TAR}"

echo "Alpine Linux rootfs setup complete at ${ROOTFS_DIR}."
