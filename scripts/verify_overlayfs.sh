#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
    echo "[-] Error: This script must be run with root privileges (e.g. sudo $0)"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_DIR}"

echo "=== Building minicontainer ==="
go build -o minicontainer .
echo "[+] minicontainer compiled successfully."

FAILED=0

echo ""
echo "=== 1. Write Isolation Test ==="
# Ensure any leftover file from earlier is cleaned up
rm -f assets/rootfs/test.txt

set +e
WRITE_OUTPUT=$(./minicontainer run --rootfs assets/rootfs /bin/sh -c 'echo "hello from overlay" > /test.txt && cat /test.txt' 2>&1)
WRITE_EXIT=$?
set -e

echo "Container output: ${WRITE_OUTPUT}"
if [ ${WRITE_EXIT} -eq 0 ] && echo "${WRITE_OUTPUT}" | grep -q "hello from overlay"; then
    if [ ! -f "assets/rootfs/test.txt" ]; then
        echo "[+] PASS: File written inside container did NOT leak to base image assets/rootfs."
    else
        echo "[-] FAIL: Leaked file found in base image assets/rootfs/test.txt!"
        FAILED=$((FAILED + 1))
    fi
else
    echo "[-] FAIL: Container write command failed."
    FAILED=$((FAILED + 1))
fi

echo ""
echo "=== 2. Delete Isolation Test ==="
# Ensure /bin/sh or /bin/busybox exists in base image before test
if [ ! -L "assets/rootfs/bin/sh" ] && [ ! -f "assets/rootfs/bin/busybox" ]; then
    echo "[-] Error: assets/rootfs/bin/sh or busybox is missing before test."
    exit 1
fi

set +e
DEL_OUTPUT=$(./minicontainer run --rootfs assets/rootfs /bin/sh -c 'rm /bin/sh && ls /bin/sh' 2>&1)
DEL_EXIT=$?
set -e

echo "Container output: ${DEL_OUTPUT}"
if echo "${DEL_OUTPUT}" | grep -iq "no such file"; then
    if [ -L "assets/rootfs/bin/sh" ]; then
        echo "[+] PASS: Deletion inside container removed file for container but preserved host base image assets/rootfs/bin/sh."
    else
        echo "[-] FAIL: Base image assets/rootfs/bin/sh was deleted!"
        FAILED=$((FAILED + 1))
    fi
else
    echo "[-] FAIL: Deletion test inside container did not behave as expected."
    FAILED=$((FAILED + 1))
fi

echo ""
echo "=== 3. Concurrent Container Test ==="
rm -f assets/rootfs/msg.txt

# Start C1 in background writing /msg.txt and holding it for 5 seconds
./minicontainer run --rootfs assets/rootfs /bin/sh -c 'echo "from C1" > /msg.txt && sleep 5' &
C1_PID=$!

# Give C1 a moment to start and write the file
sleep 1

set +e
# Start C2 simultaneously: should not see /msg.txt from C1
C2_OUTPUT=$(./minicontainer run --rootfs assets/rootfs /bin/sh -c 'cat /msg.txt 2>/dev/null || echo "not found"' 2>&1)
C2_EXIT=$?
wait ${C1_PID} 2>/dev/null || true
set -e

echo "C2 container output: ${C2_OUTPUT}"
if [ ${C2_EXIT} -eq 0 ] && echo "${C2_OUTPUT}" | grep -q "not found"; then
    echo "[+] PASS: Concurrent containers are isolated. C2 cannot see C1 writes."
else
    echo "[-] FAIL: Concurrent isolation failed. C2 saw C1's /msg.txt or error occurred."
    FAILED=$((FAILED + 1))
fi

echo ""
echo "=== 4. Cleanup Test ==="
set +e
CLEANUP_RUN=$(./minicontainer run --rootfs assets/rootfs /bin/sh -c 'echo "ephemeral" > /ephemeral.txt' 2>&1)
set -e

CONTAINERS_DIR="/var/lib/minicontainer/containers"
ACTIVE_COUNT=0
if [ -d "${CONTAINERS_DIR}" ]; then
    ACTIVE_COUNT=$(ls -1 "${CONTAINERS_DIR}" 2>/dev/null | wc -l)
fi

echo "Active container directories in ${CONTAINERS_DIR}: ${ACTIVE_COUNT}"
if [ "${ACTIVE_COUNT}" -eq 0 ]; then
    echo "[+] PASS: Container directory was completely cleaned up on exit."
else
    echo "[-] FAIL: Leftover container directories found: $(ls ${CONTAINERS_DIR})"
    FAILED=$((FAILED + 1))
fi

echo ""
echo "========================================="
if [ ${FAILED} -eq 0 ]; then
    echo "  [SUCCESS] All OverlayFS CoW isolation tests PASSED!"
    echo "========================================="
    exit 0
else
    echo "  [FAILURE] ${FAILED} test(s) failed."
    echo "========================================="
    exit 1
fi
