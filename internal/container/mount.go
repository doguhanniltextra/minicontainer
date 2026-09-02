package container

import "fmt"

const (
	procPath = "/proc"
	procType = "proc"
	devPath  = "/dev"
	tmpPath  = "/tmp"
	tmpfsType = "tmpfs"
)

// mountProc mounts the proc virtual filesystem at /proc inside the new root.
func mountProc(m mounter) error {
	return m.Mount(procType, procPath, procType, 0, "")
}

// mountDev mounts tmpfs at /dev inside the new root to isolate device nodes.
func mountDev(m mounter) error {
	return m.Mount(tmpfsType, devPath, tmpfsType, 0, "")
}

// mountTmp mounts tmpfs at /tmp inside the new root for ephemeral storage.
func mountTmp(m mounter) error {
	return m.Mount(tmpfsType, tmpPath, tmpfsType, 0, "")
}

// mountVirtualFilesystems configures all standard container virtual filesystems
// (/proc, /dev, /tmp) inside the isolated rootfs.
func mountVirtualFilesystems(m mounter) error {
	if err := mountProc(m); err != nil {
		return fmt.Errorf("mounting /proc: %w", err)
	}
	if err := mountDev(m); err != nil {
		return fmt.Errorf("mounting /dev: %w", err)
	}
	if err := mountTmp(m); err != nil {
		return fmt.Errorf("mounting /tmp: %w", err)
	}
	return nil
}

// unmountVirtualFilesystems unmounts /tmp, /dev, and /proc on container exit.
func unmountVirtualFilesystems(m mounter) error {
	var errs []error

	if err := m.Unmount(tmpPath, 0); err != nil {
		errs = append(errs, fmt.Errorf("unmounting /tmp: %w", err))
	}
	if err := m.Unmount(devPath, 0); err != nil {
		errs = append(errs, fmt.Errorf("unmounting /dev: %w", err))
	}
	if err := m.Unmount(procPath, 0); err != nil {
		errs = append(errs, fmt.Errorf("unmounting /proc: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during virtual unmounts: %v", errs)
	}
	return nil
}
