// Package container provides the core logic for creating isolated Linux
// containers using kernel namespaces.
package container

// Config holds all parameters needed to start a container.
type Config struct {
	Command  string   // the program to run inside the container, e.g. "/bin/sh"
	Args     []string // optional arguments to pass to the program
	Hostname string   // the hostname the container will see, e.g. "minicontainer"
	Rootfs   string   // path to the root filesystem directory, e.g. "assets/rootfs"

	// Resource limits (Phase 3)
	MemoryLimit int64 // bytes; 0 means unlimited (e.g. 104857600 = 100MB)
	PidsLimit   int64 // count; 0 means unlimited (e.g. 20)
	CpuQuota    int64 // microseconds per period (e.g. 50000 = 50ms = 0.5 core)
	CpuPeriod   int64 // microseconds; default 100000 (100ms)

	// Network configuration (Phase 4)
	IPAddress string // container IP with CIDR, e.g. "172.19.0.2/16"; empty means auto-allocate
	Gateway   string // gateway IP, e.g. "172.19.0.1"; empty means default gateway
	Bridge    string // bridge interface name, e.g. "mc-br0"; empty means default bridge

	// Security configuration (Phase 6)
	UserNamespace bool // enable user namespace isolation (rootless container)
	HostUID       int  // host UID to map container root (UID 0) to; default 65534 (nobody)
	HostGID       int  // host GID to map container root (GID 0) to; default 65534 (nogroup)
}

// hasLimits returns true if any resource limit is configured.
func (c Config) hasLimits() bool {
	return c.MemoryLimit > 0 || c.PidsLimit > 0 || c.CpuQuota > 0
}
