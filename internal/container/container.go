package container

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"minicontainer/internal/overlay"
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

	// ipEnvKey is the environment variable used to pass the container IP address
	// from parent (Run) to child (Init).
	ipEnvKey = "MC_IP"

	// gatewayEnvKey is the environment variable used to pass the default gateway IP
	// from parent (Run) to child (Init).
	gatewayEnvKey = "MC_GATEWAY"

	// vethEnvKey is the environment variable used to pass the container veth name
	// from parent (Run) to child (Init).
	vethEnvKey = "MC_VETH"
)

var (
	defaultIPAM *IPAM
	ipamOnce    sync.Once
)

func getIPAM() (*IPAM, error) {
	var err error
	ipamOnce.Do(func() {
		defaultIPAM, err = NewIPAM(DefaultSubnetCIDR, DefaultGatewayIP)
	})
	return defaultIPAM, err
}

// syncWaiter abstracts blocking until the parent signals that network setup is complete.
type syncWaiter interface {
	Wait() error
}

type fileSyncWaiter struct {
	file *os.File
}

func (f *fileSyncWaiter) Wait() error {
	if f.file == nil {
		return nil
	}
	defer f.file.Close()
	buf := make([]byte, 1)
	_, _ = f.file.Read(buf)
	return nil
}

type networkInitConfig struct {
	netMgr   networkManager
	syncer   syncWaiter
	contVeth string
	ip       string
	gateway  string
}

// Run starts the container by forking the current binary into new namespaces.
// The child process re-executes as "container-init" to perform namespace-internal
// setup (hostname, pivot_root, virtual mounts) before exec'ing the real command.
func Run(cfg Config) error {
	ipam, err := getIPAM()
	if err != nil {
		return fmt.Errorf("initializing IPAM: %w", err)
	}
	netMgr := newNetworkManager(realNetlinkOps{})
	store := NewDefaultContainerStore()
	overlayMnt := overlay.New()
	return runWith(cfg, realCmdRunner{}, netMgr, ipam, store, overlayMnt)
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
	contVeth := os.Getenv(vethEnvKey)
	ip := os.Getenv(ipEnvKey)
	gateway := os.Getenv(gatewayEnvKey)
	command := os.Args[2]
	commandArgs := os.Args[2:]

	var netCfgs []networkInitConfig
	if contVeth != "" {
		netCfgs = append(netCfgs, networkInitConfig{
			netMgr:   newNetworkManager(realNetlinkOps{}),
			syncer:   &fileSyncWaiter{file: os.NewFile(3, "sync-pipe")},
			contVeth: contVeth,
			ip:       ip,
			gateway:  gateway,
		})
	}

	return initWith(hostname, rootfs, command, commandArgs, realHostnamer{}, realMounter{}, realPivotRooter{}, realExecer{}, netCfgs...)
}

// runWith is the testable core of Run.
// It sets up network, cgroups, re-execs /proc/self/exe with ContainerInitArg,
// moves veth peer to the child PID, attaches to bridge, and waits for completion.
func runWith(cfg Config, runner cmdRunner, opts ...any) error {
	var cg cgroupManager
	var netMgr networkManager
	var ipam *IPAM
	var store ContainerStore
	var overlayMnt overlay.Mounter

	for _, opt := range opts {
		switch v := opt.(type) {
		case cgroupManager:
			cg = v
		case networkManager:
			netMgr = v
		case *IPAM:
			ipam = v
		case ContainerStore:
			store = v
		case overlay.Mounter:
			overlayMnt = v
		}
	}

	rootfsToPass := cfg.Rootfs
	if store != nil && overlayMnt != nil && cfg.Rootfs != "" {
		absRootfs, err := filepath.Abs(cfg.Rootfs)
		if err != nil {
			return fmt.Errorf("resolving absolute rootfs path: %w", err)
		}

		id, mergedPath, err := store.Create(absRootfs)
		if err != nil {
			return fmt.Errorf("creating container filesystem: %w", err)
		}
		defer func() {
			_ = store.Destroy(id)
		}()

		upper := store.UpperPath(id)
		work := store.WorkPath(id)
		if err := overlayMnt.Mount(absRootfs, upper, work, mergedPath); err != nil {
			return fmt.Errorf("mounting overlay filesystem: %w", err)
		}
		defer func() {
			_ = overlayMnt.Unmount(mergedPath)
		}()

		rootfsToPass = mergedPath
	}

	// Construct: /proc/self/exe container-init <command> [args...]
	args := append([]string{ContainerInitArg, cfg.Command}, cfg.Args...)
	cmd := exec.Command("/proc/self/exe", args...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	env := append(os.Environ(),
		hostnameEnvKey+"="+cfg.Hostname,
		rootfsEnvKey+"="+rootfsToPass,
	)

	var syncW *os.File
	var hostVeth, contVeth, bridgeName string
	if netMgr != nil {
		bridgeName = cfg.Bridge
		if bridgeName == "" {
			bridgeName = DefaultBridgeName
		}
		gatewayIP := cfg.Gateway
		if gatewayIP == "" {
			gatewayIP = DefaultGatewayIP
		}

		containerIP := cfg.IPAddress
		if containerIP == "" {
			if ipam != nil {
				allocIP, err := ipam.Allocate()
				if err != nil {
					return fmt.Errorf("allocating container IP: %w", err)
				}
				containerIP = allocIP
				defer ipam.Release(containerIP)
			} else {
				containerIP = "172.19.0.2/16"
			}
		}

		if err := netMgr.EnsureBridge(bridgeName, gatewayIP+"/16"); err != nil {
			return fmt.Errorf("ensuring bridge %s: %w", bridgeName, err)
		}

		if err := netMgr.EnableOutboundAccess(DefaultSubnetCIDR, bridgeName); err != nil {
			return fmt.Errorf("enabling outbound access for %s: %w", bridgeName, err)
		}

		var err error
		hostVeth, contVeth, err = GenerateVethNames()
		if err != nil {
			return fmt.Errorf("generating veth names: %w", err)
		}

		if err := netMgr.CreateVethPair(hostVeth, contVeth); err != nil {
			return fmt.Errorf("creating veth pair: %w", err)
		}

		r, w, err := os.Pipe()
		if err != nil {
			return fmt.Errorf("creating sync pipe: %w", err)
		}
		syncW = w
		cmd.ExtraFiles = []*os.File{r}

		env = append(env,
			vethEnvKey+"="+contVeth,
			ipEnvKey+"="+containerIP,
			gatewayEnvKey+"="+gatewayIP,
		)
	}

	defer func() {
		if syncW != nil {
			_ = syncW.Close()
		}
	}()

	cmd.Env = env
	cmd.SysProcAttr = buildSysProcAttrFor(cfg)

	var cgToUse cgroupManager
	if cfg.hasLimits() {
		if cg != nil {
			cgToUse = cg
		} else {
			var err error
			cgToUse, err = newCgroupManager()
			if err != nil {
				return fmt.Errorf("creating cgroup: %w", err)
			}
		}
		defer cgToUse.cleanup()
		if err := cgToUse.apply(cfg); err != nil {
			return fmt.Errorf("applying cgroup limits: %w", err)
		}
	}

	if err := runner.Start(cmd); err != nil {
		return fmt.Errorf("starting container process: %w", err)
	}

	// Close read-end of pipe in parent process
	if len(cmd.ExtraFiles) > 0 && cmd.ExtraFiles[0] != nil {
		_ = cmd.ExtraFiles[0].Close()
	}

	if cgToUse != nil {
		if err := cgToUse.addProcess(cmd.Process.Pid); err != nil {
			return fmt.Errorf("adding process to cgroup: %w", err)
		}
	}

	if netMgr != nil {
		if err := netMgr.MoveInterfaceToNetns(contVeth, cmd.Process.Pid); err != nil {
			return fmt.Errorf("moving interface %s to netns of pid %d: %w", contVeth, cmd.Process.Pid, err)
		}
		if err := netMgr.AttachToBridge(bridgeName, hostVeth); err != nil {
			return fmt.Errorf("attaching host peer %s to bridge %s: %w", hostVeth, bridgeName, err)
		}
		// Unblock child process
		_ = syncW.Close()
		syncW = nil
	}

	if err := runner.Wait(cmd); err != nil {
		return fmt.Errorf("waiting for container process: %w", err)
	}

	return nil
}

// initWith is the testable core of Init.
// It runs fully inside the child's new namespaces.
func initWith(hostname, rootfs, command string, commandArgs []string, h hostnamer, m mounter, p pivotRooter, e execer, netCfgs ...networkInitConfig) error {
	// Configure network inside container if network configuration is provided
	if len(netCfgs) > 0 && netCfgs[0].netMgr != nil && netCfgs[0].contVeth != "" {
		netCfg := netCfgs[0]
		if netCfg.syncer != nil {
			if err := netCfg.syncer.Wait(); err != nil {
				return fmt.Errorf("waiting for network sync: %w", err)
			}
		}
		if err := netCfg.netMgr.ConfigureContainerNetwork(netCfg.contVeth, netCfg.ip, netCfg.gateway); err != nil {
			return fmt.Errorf("configuring container network: %w", err)
		}
	}

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
