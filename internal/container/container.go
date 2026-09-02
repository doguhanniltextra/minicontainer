package container

import (
	"fmt"
	"os"
	"os/exec"
)

const (
	// ContainerInitArg is the sentinel argument injected into the child process
	// so it knows to run Init() instead of the CLI. Exported for use by cmd/run.go.
	ContainerInitArg = "container-init"

	// hostnameEnvKey is the environment variable used to pass the desired hostname
	// from the parent (Run) to the child (Init) across the process boundary.
	hostnameEnvKey = "MC_HOSTNAME"

	// rootfsEnvKey is the environment variable used to pass the root filesystem path
	// from the parent (Run) to the child (Init) across the process boundary.
	rootfsEnvKey = "MC_ROOTFS"
)

// Run starts the container by forking the current binary into new namespaces.
// The child process re-executes as "container-init" to perform namespace-internal
// setup (hostname, pivot_root, virtual mounts) before exec'ing the real command.
func Run(cfg Config) error {
	return runWith(cfg, realCmdRunner{})
}

// Init is invoked when the binary is re-executed as the container child.
// It runs inside the new namespaces, performs rootfs and mount setup, and execs the command.
// Never call this directly — it is called by cmd/run.go on detecting ContainerInitArg.
func Init() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("container-init requires a command argument, got args: %v", os.Args)
	}
	hostname := os.Getenv(hostnameEnvKey)
	rootfs := os.Getenv(rootfsEnvKey)
	command := os.Args[2]
	commandArgs := os.Args[2:]
	return initWith(hostname, rootfs, command, commandArgs, realHostnamer{}, realMounter{}, realPivotRooter{}, realExecer{})
}

// runWith is the testable core of Run.
// It re-execs /proc/self/exe with ContainerInitArg so the child enters its new namespaces
// before doing any setup.
func runWith(cfg Config, runner cmdRunner) error {
	// Construct: /proc/self/exe container-init <command> [args...]
	args := append([]string{ContainerInitArg, cfg.Command}, cfg.Args...)
	cmd := exec.Command("/proc/self/exe", args...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Communicate configuration to the child via environment variables.
	cmd.Env = append(os.Environ(),
		hostnameEnvKey+"="+cfg.Hostname,
		rootfsEnvKey+"="+cfg.Rootfs,
	)

	// These flags tell the kernel to create new namespaces for the child process.
	// The parent's namespaces are not affected.
	cmd.SysProcAttr = buildSysProcAttr()

	return runner.Run(cmd)
}

// initWith is the testable core of Init.
// It runs fully inside the child's new namespaces.
func initWith(hostname, rootfs, command string, commandArgs []string, h hostnamer, m mounter, p pivotRooter, e execer) error {
	// 1. Set hostname inside the child's UTS namespace
	if err := setHostname(h, hostname); err != nil {
		return fmt.Errorf("setting hostname: %w", err)
	}

	// 2. Pivot into the isolated root filesystem
	if rootfs != "" {
		if err := pivotRoot(rootfs, m, p); err != nil {
			return fmt.Errorf("pivot_root setup: %w", err)
		}
	}

	// 3. Mount virtual filesystems (/proc, /dev, /tmp) inside the new root
	if err := mountVirtualFilesystems(m); err != nil {
		return fmt.Errorf("mounting virtual filesystems: %w", err)
	}
	defer unmountVirtualFilesystems(m)

	// 4. Replace this process image with the real command
	return e.Exec(command, commandArgs, os.Environ())
}
