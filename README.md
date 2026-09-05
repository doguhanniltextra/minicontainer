# minicontainer

[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Platform](https://img.shields.io/badge/platform-linux-FCC624?style=flat&logo=linux&logoColor=black)](https://kernel.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Tests](https://img.shields.io/badge/tests-100%25%20passing-brightgreen.svg)]()

**minicontainer** is a lightweight, educational container runtime written from scratch in Go for Linux. It builds containers directly from kernel primitives without relying on Docker, containerd, or runc.

It demonstrates how real container engines work under the hood by implementing **Namespaces**, **Cgroups v2**, **OverlayFS (Copy-on-Write)**, **Bridge Networking & NAT**, and **Security Hardening** from first principles.

---

## Features

- **Linux Namespaces:**
  - `PID`: Isolated process tree (container main process is PID 1).
  - `UTS`: Isolated hostname (`--hostname`).
  - `MNT`: Isolated mount table with `pivot_root(2)` into an Alpine rootfs.
  - `NET`: Isolated network stack with a dedicated `eth0` interface and route table.
  - `USER`: Rootless container isolation (`CLONE_NEWUSER`) mapping container root (UID 0) to host unprivileged user (UID 65534 `nobody`).

- **Copy-on-Write Storage (OverlayFS):**
  - Read-only base images (`lowerdir`) remain untouched.
  - Per-container ephemeral writable layers (`upperdir`, `workdir`, `mergeddir`).
  - Write/delete isolation: multiple containers can safely share the same base image concurrently.
  - Automatic cleanup: container directories are deleted immediately on container exit.

- **Resource Limits (Cgroups v2):**
  - Memory limit (`-m` / `--memory`): e.g. `100m`, `512M`, `1g`.
  - CPU quota (`-c` / `--cpus`): e.g. `0.5`, `2.0` cores via `cpu.max`.
  - Process limit (`-p` / `--pids`): protection against fork bombs via `pids.max`.

- **Container Networking:**
  - Custom Linux bridge (`mc-br0`) and veth pairs (`mc-h-*` and `mc-c-*`).
  - Interface transfer into container network namespace and rename to `eth0`.
  - Internal IPAM (IP Address Management) supporting dynamic IP allocation and reclamation.
  - Outbound internet access via kernel IP forwarding and `iptables` MASQUERADE rules.

- **Clean Architecture & TDD:**
  - Built strictly following **SOLID** and **DRY** principles.
  - Syscall and kernel interactions are abstracted behind testable interfaces.
  - **100% of unit tests run without root privileges!**

---

## Architecture

```text
Host (Run)
│
├── 1. IPAM & Network Setup
│      ├── Ensure Bridge (mc-br0: 172.19.0.1/16)
│      ├── Create Veth Pair (mc-h-* <-> mc-c-*)
│      └── Attach mc-h-* to Bridge & Enable NAT MASQUERADE
│
├── 2. OverlayFS Layer Preparation
│      ├── Allocate /var/lib/minicontainer/containers/{id}/(upper|work|merged)
│      └── Mount OverlayFS (lowerdir=assets/rootfs, upperdir, workdir -> mergeddir)
│
├── 3. Cgroups v2 Setup
│      └── /sys/fs/cgroup/minicontainer-{id}/ (memory.max, pids.max, cpu.max)
│
└── 4. Spawn Child Process (/proc/self/exe container-init)
       │
       ├── SysProcAttr: CLONE_NEWPID | CLONE_NEWUTS | CLONE_NEWNET | CLONE_NEWNS | CLONE_NEWUSER
       │
       └── Container Child (Init)
           ├── Receive veth via Sync Pipe -> Rename to eth0, Assign IP & Default Route
           ├── Set Hostname
           ├── pivot_root(mergeddir, .old_root) & Unmount .old_root
           ├── Mount Virtual Filesystems (/proc, /dev, /tmp)
           └── execvp(command, args...)
```

---

## Prerequisites

- **Operating System:** Linux (Kernel 5.8+ recommended with Cgroups v2 enabled).
- **WSL 2:** Supported (Ubuntu 20.04/22.04/24.04).
- **Go:** Version 1.22 or higher.
- **Dependencies:**
  - `iptables` (for outbound NAT access)
  - `iproute2` (`ip` command)
  - `curl` or `wget` (to download the Alpine rootfs)

---

## Quick Start

### 1. Clone the Repository
```bash
git clone https://github.com/doguhanniltextra/minicontainer.git
cd minicontainer
```

### 2. Prepare the Alpine Linux Root Filesystem
Download and extract a minimal Alpine rootfs into `assets/rootfs/`:
```bash
./scripts/setup_rootfs.sh
```

### 3. Build the Binary
```bash
go build -o minicontainer .
```

### 4. Run Your First Container
```bash
sudo ./minicontainer run /bin/sh
```
Inside the container, test isolation:
```sh
/ # ps aux
PID   USER     TIME  COMMAND
    1 root      0:00 /bin/sh
    4 root      0:00 ps aux

/ # ip addr show eth0
2: eth0@if3: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 ...
    inet 172.19.0.2/16 scope global eth0

/ # ping -c 2 8.8.8.8
64 bytes from 8.8.8.8: seq=0 ttl=115 time=12.4 ms
```

---

## CLI Usage & Flags

```text
Usage:
  minicontainer run [--rootfs <path>] [-m|--memory <limit>] [-p|--pids <limit>] [-c|--cpus <cores>] [-u|--user-namespace] <command> [args...]
```

### Available Options

| Flag | Short | Description | Default |
|---|---|---|---|
| `--rootfs <path>` | `-r` | Path to root filesystem | `assets/rootfs` |
| `--memory <limit>` | `-m` | Memory limit (e.g. `50m`, `512M`, `1g`) | Unlimited (`0`) |
| `--pids <limit>` | `-p` | Maximum number of processes (PIDs) | Unlimited (`0`) |
| `--cpus <cores>` | `-c` | Maximum CPU cores (e.g. `0.5`, `1.5`) | Unlimited (`0`) |
| `--user-namespace` | `-u` | Enable User Namespace (maps root to `nobody:65534`) | `false` |
| `--help` | `-h` | Display help message | — |

---

## Examples

### Resource-Constrained Container
Limit memory to 100MB and CPU to half a core:
```bash
sudo ./minicontainer run -m 100m -c 0.5 /bin/sh
```

### Fork Bomb Protection
Limit process count to 20 PIDs:
```bash
sudo ./minicontainer run -p 20 /bin/sh
```

### Rootless Container (User Namespace)
Run with user namespace isolation:
```bash
sudo ./minicontainer run -u /bin/sh
```

### Copy-on-Write (CoW) Safety Test
Deleting files inside the container will **never** alter the host base image:
```bash
sudo ./minicontainer run /bin/sh -c "rm -rf /bin && ls /bin"
# Host check: assets/rootfs/bin remains completely intact!
ls assets/rootfs/bin
```

---

## Testing & Verification

### Run Unit Tests (No Root Required)
All unit tests mock syscalls, Netlink, and filesystem operations:
```bash
go test -v ./...
```

### Run Live End-to-End Verification Scripts

Verify Cgroups v2 resource limits (RAM, CPU, PID):
```bash
sudo ./scripts/verify_limits.sh
```

Verify container networking (loopback, bridge, host-to-container ping, NAT):
```bash
sudo ./scripts/verify_network.sh
```

Verify OverlayFS Copy-on-Write isolation and cleanup:
```bash
sudo ./scripts/verify_overlayfs.sh
```

---

## Project Structure

```text
.
├── assets/
│   └── rootfs/               # Base Alpine Linux root filesystem
├── cmd/
│   ├── run.go                # Cobra CLI commands & flag parsing
│   └── run_test.go           # CLI argument parsing unit tests
├── internal/
│   ├── container/            # Core container lifecycle
│   │   ├── cgroup.go         # Cgroups v2 manager (memory, pids, cpu)
│   │   ├── config.go         # Container configuration struct
│   │   ├── container.go      # Run() and Init() lifecycle orchestration
│   │   ├── ipam.go           # Thread-safe IP address allocator
│   │   ├── mount.go          # Virtual mounts (/proc, /dev, /tmp)
│   │   ├── namespace.go      # Namespace setup (Cloneflags & UID/GID maps)
│   │   ├── network.go        # Bridge, veth, and NAT manager
│   │   ├── rootfs.go         # pivot_root(2) isolation
│   │   ├── store.go          # Per-container overlay directory store
│   │   └── syscalls.go       # Abstracted interfaces for syscall mocking
│   ├── image/                # Readonly base image store
│   │   ├── image.go          # Image store interface & metadata
│   │   └── local_store.go    # Local filesystem image repository
│   └── overlay/              # OverlayFS mount manager
│       └── overlay.go        # Kernel overlayfs mount/unmount abstraction
├── scripts/
│   ├── setup_rootfs.sh       # Downloads & unpacks Alpine rootfs
│   ├── verify_limits.sh      # Live verification for Cgroups limits
│   ├── verify_network.sh     # Live verification for network & NAT
│   └── verify_overlayfs.sh   # Live verification for OverlayFS isolation
├── main.go                   # Application entrypoint
└── README.md
```

---

## Roadmap & Phases

- [x] **Phase 1: Namespaces** — PID, UTS, Mount namespaces.
- [x] **Phase 2: Rootfs & Filesystem** — `pivot_root(2)` into isolated rootfs.
- [x] **Phase 3: Cgroups v2** — Memory, CPU, and PID limits.
- [x] **Phase 4: Networking** — Bridge, Veth pairs, IPAM, and iptables NAT.
- [x] **Phase 5: Image System & OverlayFS** — Copy-on-Write layers & automatic cleanup.
- [ ] **Phase 6: Security Hardening** — User namespace (rootless), capability drops, seccomp BPF filters, and masked paths.

---

## License

MIT License. See [LICENSE](LICENSE) for details.
