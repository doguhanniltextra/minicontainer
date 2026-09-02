package container

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const (
	// oldRootName is the temporary directory name inside the new rootfs
	// where the old host root is placed during pivot_root.
	oldRootName = ".old_root"
)

// pivotRoot isolates the filesystem of the container process by swapping
// the current root filesystem with the new rootfs directory using pivot_root(2).
//
// Sequence:
// 1. Make mount namespace private (MS_REC | MS_PRIVATE) to prevent mount leaks to the host.
// 2. Bind-mount newRoot to itself so it qualifies as an independent mount point.
// 3. Create newRoot/.old_root directory to hold the previous root filesystem.
// 4. Invoke syscall.PivotRoot(newRoot, newRoot/.old_root).
// 5. Change current directory to "/".
// 6. Unmount "/.old_root" (MNT_DETACH) and remove the temporary directory.
func pivotRoot(newRoot string, m mounter, p pivotRooter) error {
	// 1. Prevent mounts in this namespace from propagating back to the host
	if err := m.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("making mounts private: %w", err)
	}

	// 2. pivot_root requires newRoot to be a mount point; bind mount it onto itself
	if err := m.Mount(newRoot, newRoot, "bind", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind mounting new root: %w", err)
	}

	// 3. Create destination directory for the old root
	oldRootPath := filepath.Join(newRoot, oldRootName)
	if err := os.MkdirAll(oldRootPath, 0700); err != nil {
		return fmt.Errorf("creating old_root directory: %w", err)
	}

	// 4. Swap root mount points
	if err := p.PivotRoot(newRoot, oldRootPath); err != nil {
		return fmt.Errorf("pivot_root: %w", err)
	}

	// 5. Switch process working directory to the new root
	if err := p.Chdir("/"); err != nil {
		return fmt.Errorf("changing directory to new root: %w", err)
	}

	// 6. Detach old root and remove temporary mount folder
	oldRootMount := "/" + oldRootName
	if err := m.Unmount(oldRootMount, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("unmounting old root: %w", err)
	}

	_ = os.Remove(oldRootMount)

	return nil
}
