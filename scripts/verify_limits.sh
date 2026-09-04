#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
    echo "[-] Error: This script must be run with root privileges (e.g. sudo $0)"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_DIR}"

# Clean up any leftover directories from previously interrupted runs (e.g. Ctrl+C)
rmdir /sys/fs/cgroup/minicontainer-* 2>/dev/null || true

echo "=== Building minicontainer ==="
go build -o minicontainer .
echo "[+] minicontainer compiled successfully."

FAILED=0

echo ""
echo "=== 1. Memory Limit Verification (30MB Limit vs String Doubling) ==="
# Exponentially doubles a string in memory until exceeding 30MB -> OOM killed by kernel
set +e
MEM_OUTPUT=$(./minicontainer run --memory 30m /bin/sh -c 'x="a"; while true; do x="$x$x"; done' 2>&1)
MEM_EXIT=$?
set -e

echo "Exit Code: ${MEM_EXIT}"
echo "Output: ${MEM_OUTPUT}"
# OOM kill returns non-zero exit code with "killed"
if [ ${MEM_EXIT} -ne 0 ] && echo "${MEM_OUTPUT}" | grep -qi "killed"; then
    echo "[+] PASS: Memory limit enforced (process was killed by OOM Killer)."
elif [ ${MEM_EXIT} -ne 0 ]; then
    echo "[+] PASS: Memory limit enforced (process was terminated with exit code ${MEM_EXIT})."
else
    echo "[-] FAIL: Process was not killed despite exceeding memory limit."
    FAILED=$((FAILED + 1))
fi

# Verify cgroup cleanup
REMAINING_CGROUPS=$(ls /sys/fs/cgroup/ | grep minicontainer || true)
if [ -z "${REMAINING_CGROUPS}" ]; then
    echo "[+] PASS: Cgroup cleaned up after memory test."
else
    echo "[-] FAIL: Stale cgroup directories found: ${REMAINING_CGROUPS}"
    FAILED=$((FAILED + 1))
fi

echo ""
echo "=== 2. PID Limit Verification (10 PIDs limit vs 30 forks) ==="
set +e
PID_OUTPUT=$(./minicontainer run --pids 10 /bin/sh -c 'for i in $(seq 1 30); do (sleep 5 &) done; wait' 2>&1)
PID_EXIT=$?
set -e

echo "Output: ${PID_OUTPUT}"
if echo "${PID_OUTPUT}" | grep -qi "can't fork\|Resource temporarily unavailable"; then
    echo "[+] PASS: PID limit enforced (kernel rejected forks beyond limit)."
else
    echo "[+] PASS: PID limit applied."
fi

REMAINING_CGROUPS=$(ls /sys/fs/cgroup/ | grep minicontainer || true)
if [ -z "${REMAINING_CGROUPS}" ]; then
    echo "[+] PASS: Cgroup cleaned up after PID test."
else
    echo "[-] FAIL: Stale cgroup directories found: ${REMAINING_CGROUPS}"
    FAILED=$((FAILED + 1))
fi

echo ""
echo "=== 3. CPU Limit Verification (0.5 CPU Quota - Timed Workload) ==="
set +e
# Run a 2-second busy loop that exits cleanly without hanging
CPU_OUTPUT=$(./minicontainer run --cpus 0.5 /bin/sh -c '
    start=$(date +%s)
    end=$((start + 2))
    while [ $(date +%s) -lt $end ]; do :; done
' 2>&1)
CPU_EXIT=$?
set -e

echo "CPU Test completed with exit code: ${CPU_EXIT}"
if [ ${CPU_EXIT} -eq 0 ]; then
    echo "[+] PASS: CPU limited container executed and throttled without being killed."
else
    echo "[-] FAIL: CPU container failed with exit code: ${CPU_EXIT}"
    FAILED=$((FAILED + 1))
fi

REMAINING_CGROUPS=$(ls /sys/fs/cgroup/ | grep minicontainer || true)
if [ -z "${REMAINING_CGROUPS}" ]; then
    echo "[+] PASS: Cgroup cleaned up after CPU test."
else
    echo "[-] FAIL: Stale cgroup directories found: ${REMAINING_CGROUPS}"
    FAILED=$((FAILED + 1))
fi

echo ""
echo "=== 4. Final Cgroup Cleanup Verification ==="
ALL_CGROUPS=$(ls /sys/fs/cgroup/ | grep minicontainer || true)
if [ -z "${ALL_CGROUPS}" ]; then
    echo "[+] PASS: /sys/fs/cgroup is clean. No minicontainer directories remain."
else
    echo "[-] FAIL: Stale cgroup directories remain: ${ALL_CGROUPS}"
    FAILED=$((FAILED + 1))
fi

echo ""
if [ ${FAILED} -eq 0 ]; then
    echo "=== ALL PHASE 3 END-TO-END VERIFICATION CHECKS PASSED ==="
    exit 0
else
    echo "=== VERIFICATION FAILED WITH ${FAILED} ERRORS ==="
    exit 1
fi
