package overlay

import (
	"fmt"
	"syscall"
)

// Mounter mounts and unmounts an OverlayFS at the given merged directory.
type Mounter interface {
	// Mount mounts an OverlayFS to mergeddir using lowerdir, upperdir, and workdir.
	Mount(lowerdir, upperdir, workdir, mergeddir string) error
	// Unmount unmounts the OverlayFS filesystem from mergeddir.
	Unmount(mergeddir string) error
}

// mountSyscaller abstracts syscall.Mount and syscall.Unmount for unit testing without root.
type mountSyscaller interface {
	Mount(source, target, fstype string, flags uintptr, data string) error
	Unmount(target string, flags int) error
}

type realMountSyscaller struct{}

func (realMountSyscaller) Mount(source, target, fstype string, flags uintptr, data string) error {
	return syscall.Mount(source, target, fstype, flags, data)
}

func (realMountSyscaller) Unmount(target string, flags int) error {
	return syscall.Unmount(target, flags)
}

type overlayMounter struct {
	mnt mountSyscaller
}

// New creates a new Mounter using the host's Linux mount syscalls.
func New() Mounter {
	return newWithSyscaller(realMountSyscaller{})
}

// newWithSyscaller creates an overlayMounter with an injected mountSyscaller implementation.
func newWithSyscaller(mnt mountSyscaller) *overlayMounter {
	return &overlayMounter{mnt: mnt}
}

// Mount mounts an OverlayFS to mergeddir using lowerdir, upperdir, and workdir.
func (o *overlayMounter) Mount(lowerdir, upperdir, workdir, mergeddir string) error {
	if lowerdir == "" || upperdir == "" || workdir == "" || mergeddir == "" {
		return fmt.Errorf("all directory paths (lowerdir, upperdir, workdir, mergeddir) are required")
	}

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerdir, upperdir, workdir)
	if err := o.mnt.Mount("overlay", mergeddir, "overlay", 0, opts); err != nil {
		return fmt.Errorf("mounting overlayfs to %s: %w", mergeddir, err)
	}

	return nil
}

// Unmount unmounts the OverlayFS from mergeddir.
func (o *overlayMounter) Unmount(mergeddir string) error {
	if mergeddir == "" {
		return fmt.Errorf("mergeddir cannot be empty")
	}

	if err := o.mnt.Unmount(mergeddir, 0); err != nil {
		return fmt.Errorf("unmounting overlayfs from %s: %w", mergeddir, err)
	}

	return nil
}
