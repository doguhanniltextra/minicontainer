// Package container provides the core logic for creating isolated Linux
// containers using kernel namespaces.
package container

// Config holds all parameters needed to start a container.
type Config struct {
	Command  string   // the program to run inside the container, e.g. "/bin/sh"
	Args     []string // optional arguments to pass to the program
	Hostname string   // the hostname the container will see, e.g. "minicontainer"
	Rootfs   string   // path to the root filesystem directory, e.g. "assets/rootfs"
}
