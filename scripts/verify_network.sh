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
echo "=== 1. Loopback Interface Verification ==="
set +e
LO_OUTPUT=$(./minicontainer run --rootfs assets/rootfs /bin/sh -c 'ip link show lo && ping -c 2 127.0.0.1' 2>&1)
LO_EXIT=$?
set -e

echo "${LO_OUTPUT}"
if [ ${LO_EXIT} -eq 0 ] && echo "${LO_OUTPUT}" | grep -q "state UP" && echo "${LO_OUTPUT}" | grep -q "0% packet loss"; then
    echo "[+] PASS: Loopback interface is UP and responsive."
else
    echo "[-] FAIL: Loopback verification failed."
    FAILED=$((FAILED + 1))
fi

echo ""
echo "=== 2. Host Bridge Connectivity Verification ==="
set +e
BRIDGE_OUTPUT=$(./minicontainer run --rootfs assets/rootfs /bin/sh -c 'ip addr show eth0 && ip route show && ping -c 2 172.19.0.1' 2>&1)
BRIDGE_EXIT=$?
set -e

echo "${BRIDGE_OUTPUT}"
if [ ${BRIDGE_EXIT} -eq 0 ] && echo "${BRIDGE_OUTPUT}" | grep -q "172.19.0." && echo "${BRIDGE_OUTPUT}" | grep -q "default via 172.19.0.1" && echo "${BRIDGE_OUTPUT}" | grep -q "0% packet loss"; then
    echo "[+] PASS: Container received IP, default route installed, and host bridge pingable."
else
    echo "[-] FAIL: Host bridge connectivity verification failed."
    FAILED=$((FAILED + 1))
fi

echo ""
echo "=== 3. Host-to-Container Ping Verification ==="
set +e
# Start container in background sleeping 5 seconds
./minicontainer run --rootfs assets/rootfs /bin/sh -c 'sleep 5' &
CONT_PID=$!
sleep 1

HOST_PING=$(ping -c 2 172.19.0.2 2>&1)
PING_EXIT=$?
wait ${CONT_PID} 2>/dev/null || true
set -e

echo "${HOST_PING}"
if [ ${PING_EXIT} -eq 0 ] && echo "${HOST_PING}" | grep -q "0% packet loss"; then
    echo "[+] PASS: Host successfully pinged container IP (172.19.0.2)."
else
    echo "[-] FAIL: Host cannot ping container IP."
    FAILED=$((FAILED + 1))
fi

echo ""
echo "=== 4. Inter-Container Communication Verification ==="
set +e
# Start Container 1 listening on netcat in background
./minicontainer run --rootfs assets/rootfs /bin/sh -c 'nc -l -p 9000 > /tmp/nc_received.txt' &
C1_PID=$!
sleep 1

# Start Container 2 to ping C1 and send payload via netcat
C2_OUTPUT=$(./minicontainer run --rootfs assets/rootfs /bin/sh -c 'ping -c 2 172.19.0.2 && echo "Hello from C2" | nc 172.19.0.2 9000' 2>&1)
C2_EXIT=$?
wait ${C1_PID} 2>/dev/null || true
set -e

echo "${C2_OUTPUT}"
if [ ${C2_EXIT} -eq 0 ] && echo "${C2_OUTPUT}" | grep -q "0% packet loss"; then
    echo "[+] PASS: Inter-container ping and communication succeeded."
else
    echo "[-] FAIL: Inter-container communication failed."
    FAILED=$((FAILED + 1))
fi

echo ""
echo "=== 5. Outbound Internet Connectivity Verification ==="
set +e
# Ping public DNS (8.8.8.8) or 1.1.1.1
OUTBOUND_OUTPUT=$(./minicontainer run --rootfs assets/rootfs /bin/sh -c 'ping -c 2 8.8.8.8 && wget -qO- http://example.com | grep -i "Example Domain"' 2>&1)
OUTBOUND_EXIT=$?
set -e

echo "${OUTBOUND_OUTPUT}"
if [ ${OUTBOUND_EXIT} -eq 0 ] && echo "${OUTBOUND_OUTPUT}" | grep -q "Example Domain"; then
    echo "[+] PASS: Outbound internet connectivity verified (ping 8.8.8.8 and wget example.com succeeded)."
else
    echo "[!] Notice: Outbound ping/wget returned exit code ${OUTBOUND_EXIT} (check host upstream connection or DNS if in restricted environment)."
fi

echo ""
echo "=== 6. Automatic Cleanup Verification ==="
REMAINING_VETHS=$(ip link show | grep "mc-h-" || true)
if [ -z "${REMAINING_VETHS}" ]; then
    echo "[+] PASS: All mc-h-* interfaces cleaned up automatically."
else
    echo "[-] FAIL: Stale veth interfaces found: ${REMAINING_VETHS}"
    FAILED=$((FAILED + 1))
fi

echo ""
if [ ${FAILED} -eq 0 ]; then
    echo "=========================================="
    echo "🎉 ALL NETWORK VERIFICATION CHECKS PASSED!"
    echo "=========================================="
    exit 0
else
    echo "=========================================="
    echo "❌ ${FAILED} CHECKS FAILED."
    echo "=========================================="
    exit 1
fi
