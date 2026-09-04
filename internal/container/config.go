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
}
