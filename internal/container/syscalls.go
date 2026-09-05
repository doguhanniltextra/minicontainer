package container

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// fsWriter abstracts filesystem write operations used by the cgroup manager.
// Tests can inject a mock to avoid requiring root privileges or real cgroups.
type fsWriter interface {
	MkdirAll(path string, perm fs.FileMode) error
	WriteFile(name string, data []byte, perm fs.FileMode) error
	Remove(name string) error
	RemoveAll(path string) error
}

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
// The real implementation calls (*exec.Cmd).Run(), (*exec.Cmd).Start(), and (*exec.Cmd).Wait().
// Tests can inject a mock to avoid spawning actual child processes.
type cmdRunner interface {
	Run(cmd *exec.Cmd) error
	Start(cmd *exec.Cmd) error
	Wait(cmd *exec.Cmd) error
}

// cgroupManager abstracts the lifecycle and limit management of a cgroup.
// Tests can inject a mock to track cgroup operations without root privileges.
type cgroupManager interface {
	apply(cfg Config) error
	addProcess(pid int) error
	cleanup() error
}

// execer abstracts syscall.Exec so Init() can be fully unit tested
// without actually replacing the running process image.
type execer interface {
	Exec(argv0 string, argv []string, envv []string) error
}

// netlinkOps abstracts the underlying kernel netlink and network device operations.
// Tests inject a mock implementation so zero root privileges are needed.
type netlinkOps interface {
	LinkExists(name string) (bool, error)
	CreateBridge(name string) error
	AddAddress(ifName string, ipCIDR string) error
	SetLinkUp(ifName string) error
	SetLinkDown(ifName string) error
	SetLinkMaster(ifName, masterName string) error
	DeleteLink(name string) error
	CreateVethPair(hostVeth, contVeth string) error
	SetLinkNetnsByPID(ifName string, pid int) error
	RenameLink(oldName, newName string) error
	AddDefaultRoute(gatewayIP, ifName string) error
}

// networkManager abstracts end-to-end container networking setup.
// Tests can inject a mock to verify the lifecycle sequencing without root privileges.
type networkManager interface {
	EnsureBridge(bridgeName, ipCIDR string) error
	CreateVethPair(hostVeth, contVeth string) error
	AttachToBridge(bridgeName, ifName string) error
	MoveInterfaceToNetns(ifName string, pid int) error
	ConfigureContainerNetwork(contVeth, ipCIDR, gatewayIP string) error
	EnableOutboundAccess(subnetCIDR, bridgeName string) error
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

func (r realCmdRunner) Start(cmd *exec.Cmd) error {
	return cmd.Start()
}

func (r realCmdRunner) Wait(cmd *exec.Cmd) error {
	return cmd.Wait()
}

// realExecer is the production implementation of execer.
// It delegates directly to syscall.Exec, which replaces the current process image.
type realExecer struct{}

func (r realExecer) Exec(argv0 string, argv []string, envv []string) error {
	return syscall.Exec(argv0, argv, envv)
}

// realFsWriter is the production implementation of fsWriter.
// It delegates directly to os.MkdirAll, os.WriteFile, and os.Remove.
type realFsWriter struct{}

func (r realFsWriter) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (r realFsWriter) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (r realFsWriter) Remove(name string) error {
	return os.Remove(name)
}

func (r realFsWriter) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// realNetlinkOps is the production implementation of netlinkOps.
// It delegates to the net standard library package and the Linux ip CLI tool.
type realNetlinkOps struct{}

func (r realNetlinkOps) LinkExists(name string) (bool, error) {
	_, err := net.InterfaceByName(name)
	if err == nil {
		return true, nil
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) || strings.Contains(err.Error(), "no such network interface") {
		return false, nil
	}
	return false, err
}

func (r realNetlinkOps) CreateBridge(name string) error {
	out, err := exec.Command("ip", "link", "add", name, "type", "bridge").CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip link add %s type bridge failed: %w (output: %s)", name, err, string(out))
	}
	return nil
}

func (r realNetlinkOps) AddAddress(ifName string, ipCIDR string) error {
	out, err := exec.Command("ip", "addr", "add", ipCIDR, "dev", ifName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip addr add %s dev %s failed: %w (output: %s)", ipCIDR, ifName, err, string(out))
	}
	return nil
}

func (r realNetlinkOps) SetLinkUp(ifName string) error {
	out, err := exec.Command("ip", "link", "set", ifName, "up").CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip link set %s up failed: %w (output: %s)", ifName, err, string(out))
	}
	return nil
}

func (r realNetlinkOps) SetLinkDown(ifName string) error {
	out, err := exec.Command("ip", "link", "set", ifName, "down").CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip link set %s down failed: %w (output: %s)", ifName, err, string(out))
	}
	return nil
}

func (r realNetlinkOps) SetLinkMaster(ifName, masterName string) error {
	out, err := exec.Command("ip", "link", "set", ifName, "master", masterName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip link set %s master %s failed: %w (output: %s)", ifName, masterName, err, string(out))
	}
	return nil
}

func (r realNetlinkOps) DeleteLink(name string) error {
	out, err := exec.Command("ip", "link", "delete", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip link delete %s failed: %w (output: %s)", name, err, string(out))
	}
	return nil
}

func (r realNetlinkOps) CreateVethPair(hostVeth, contVeth string) error {
	out, err := exec.Command("ip", "link", "add", hostVeth, "type", "veth", "peer", "name", contVeth).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip link add %s type veth peer name %s failed: %w (output: %s)", hostVeth, contVeth, err, string(out))
	}
	return nil
}

func (r realNetlinkOps) SetLinkNetnsByPID(ifName string, pid int) error {
	out, err := exec.Command("ip", "link", "set", ifName, "netns", strconv.Itoa(pid)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip link set %s netns %d failed: %w (output: %s)", ifName, pid, err, string(out))
	}
	return nil
}

func (r realNetlinkOps) RenameLink(oldName, newName string) error {
	out, err := exec.Command("ip", "link", "set", oldName, "name", newName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip link set %s name %s failed: %w (output: %s)", oldName, newName, err, string(out))
	}
	return nil
}

func (r realNetlinkOps) AddDefaultRoute(gatewayIP, ifName string) error {
	out, err := exec.Command("ip", "route", "add", "default", "via", gatewayIP, "dev", ifName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip route add default via %s dev %s failed: %w (output: %s)", gatewayIP, ifName, err, string(out))
	}
	return nil
}

// iptablesRunner abstracts executing iptables commands for NAT and packet filtering.
type iptablesRunner interface {
	RunCommand(args ...string) error
	CheckRule(args ...string) (bool, error)
}

// realIptablesRunner is the production implementation of iptablesRunner.
// It executes the host iptables CLI binary.
type realIptablesRunner struct{}

func (r realIptablesRunner) RunCommand(args ...string) error {
	out, err := exec.Command("iptables", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables %s failed: %w (output: %s)", strings.Join(args, " "), err, string(out))
	}
	return nil
}

func (r realIptablesRunner) CheckRule(args ...string) (bool, error) {
	cmd := exec.Command("iptables", args...)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Exit code 1 indicates rule does not exist
			return false, nil
		}
		return false, fmt.Errorf("iptables check %s failed: %w", strings.Join(args, " "), err)
	}
	return true, nil
}




