package container

import (
	"os/exec"
	"syscall"
)

// hostnamer abstracts the syscall needed to set the hostname.
// The real implementation calls syscall.Sethostname directly.
// Tests can inject a mock to avoid requiring root privileges.
type hostnamer interface {
	Sethostname(name []byte) error
}

// mounter abstracts the syscall operations needed to manage mounts.
// The real implementation calls syscall.Mount and syscall.Unmount directly.
// Tests can inject a mock to avoid requiring root privileges.
type mounter interface {
	Mount(source, target, fstype string, flags uintptr, data string) error
	Unmount(target string, flags int) error
}

// pivotRooter abstracts the syscall operations needed for pivot_root and directory changes.
// Tests can inject a mock to avoid requiring CAP_SYS_ADMIN privileges.
type pivotRooter interface {
	PivotRoot(newRoot, putOld string) error
	Chdir(dir string) error
}

// cmdRunner abstracts executing an external command.
// The real implementation calls (*exec.Cmd).Run().
// Tests can inject a mock to avoid spawning actual child processes.
type cmdRunner interface {
	Run(cmd *exec.Cmd) error
}

// execer abstracts syscall.Exec so Init() can be fully unit tested
// without actually replacing the running process image.
type execer interface {
	Exec(argv0 string, argv []string, envv []string) error
}

// realHostnamer is the production implementation of hostnamer.
// It delegates directly to the syscall package.
type realHostnamer struct{}

func (r realHostnamer) Sethostname(name []byte) error {
	return syscall.Sethostname(name)
}

// realMounter is the production implementation of mounter.
// It delegates directly to the syscall package.
type realMounter struct{}

func (r realMounter) Mount(source, target, fstype string, flags uintptr, data string) error {
	return syscall.Mount(source, target, fstype, flags, data)
}

func (r realMounter) Unmount(target string, flags int) error {
	return syscall.Unmount(target, flags)
}

// realPivotRooter is the production implementation of pivotRooter.
// It delegates directly to syscall.PivotRoot and syscall.Chdir.
type realPivotRooter struct{}

func (r realPivotRooter) PivotRoot(newRoot, putOld string) error {
	return syscall.PivotRoot(newRoot, putOld)
}

func (r realPivotRooter) Chdir(dir string) error {
	return syscall.Chdir(dir)
}

// realCmdRunner is the production implementation of cmdRunner.
// It delegates directly to os/exec.
type realCmdRunner struct{}

func (r realCmdRunner) Run(cmd *exec.Cmd) error {
	return cmd.Run()
}

// realExecer is the production implementation of execer.
// It delegates directly to syscall.Exec, which replaces the current process image.
type realExecer struct{}

func (r realExecer) Exec(argv0 string, argv []string, envv []string) error {
	return syscall.Exec(argv0, argv, envv)
}
