package container

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const (
	// DefaultBridgeName is the default Linux bridge used by minicontainer.
	DefaultBridgeName = "mc-br0"

	// LoopbackInterface is the standard loopback interface name.
	LoopbackInterface = "lo"

	// ContainerInterface is the standard container-side network interface name.
	ContainerInterface = "eth0"
)

// BridgeManager manages host-side Linux bridge creation, configuration, and attachment.
type BridgeManager struct {
	nl netlinkOps
}

// NewBridgeManager creates a new BridgeManager with the provided netlink operations.
func NewBridgeManager(nl netlinkOps) *BridgeManager {
	return &BridgeManager{nl: nl}
}

// EnsureBridge checks whether the specified bridge device exists.
// If not, it creates the bridge and assigns the given gateway IP with CIDR.
// It ensures the bridge interface is set to UP.
func (b *BridgeManager) EnsureBridge(name, ipCIDR string) error {
	exists, err := b.nl.LinkExists(name)
	if err != nil {
		return fmt.Errorf("checking bridge %s existence: %w", name, err)
	}

	if !exists {
		if err := b.nl.CreateBridge(name); err != nil {
			return fmt.Errorf("creating bridge %s: %w", name, err)
		}
		if err := b.nl.AddAddress(name, ipCIDR); err != nil {
			return fmt.Errorf("assigning IP %s to bridge %s: %w", ipCIDR, name, err)
		}
	}

	if err := b.nl.SetLinkUp(name); err != nil {
		return fmt.Errorf("setting bridge %s up: %w", name, err)
	}

	return nil
}

// AttachToBridge sets the master of the specified network interface (e.g. host-side veth)
// to the given bridge device, effectively connecting it to the bridge network.
func (b *BridgeManager) AttachToBridge(bridgeName, ifName string) error {
	if err := b.nl.SetLinkMaster(ifName, bridgeName); err != nil {
		return fmt.Errorf("attaching %s to bridge %s: %w", ifName, bridgeName, err)
	}
	return nil
}

// GenerateVethNames creates a pair of short, unique interface names
// adhering to the Linux 15-character interface name limit (IFNAMSIZ - 1).
// Returns (hostVethName, contVethName, error).
// Example: ("mc-h-a1b2c3", "mc-c-a1b2c3", nil)
func GenerateVethNames() (string, string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generating random veth suffix: %w", err)
	}
	suffix := hex.EncodeToString(b)
	return "mc-h-" + suffix, "mc-c-" + suffix, nil
}

// VethManager manages veth pair lifecycle and namespace migration.
type VethManager struct {
	nl netlinkOps
}

// NewVethManager creates a new VethManager with the provided netlink operations.
func NewVethManager(nl netlinkOps) *VethManager {
	return &VethManager{nl: nl}
}

// CreateVethPair creates the virtual ethernet pair in the host network namespace.
func (v *VethManager) CreateVethPair(hostVeth, contVeth string) error {
	if err := v.nl.CreateVethPair(hostVeth, contVeth); err != nil {
		return fmt.Errorf("creating veth pair (%s, %s): %w", hostVeth, contVeth, err)
	}
	return nil
}

// MoveToNetns moves the container peer interface into the target PID's network namespace.
func (v *VethManager) MoveToNetns(ifName string, pid int) error {
	if err := v.nl.SetLinkNetnsByPID(ifName, pid); err != nil {
		return fmt.Errorf("moving interface %s to netns of pid %d: %w", ifName, pid, err)
	}
	return nil
}

// SetupHostPeer attaches the host-side veth peer to the bridge and sets its link state to UP.
func (v *VethManager) SetupHostPeer(hostVeth, bridgeName string, bm *BridgeManager) error {
	if err := bm.AttachToBridge(bridgeName, hostVeth); err != nil {
		return fmt.Errorf("attaching host peer %s to bridge %s: %w", hostVeth, bridgeName, err)
	}
	if err := v.nl.SetLinkUp(hostVeth); err != nil {
		return fmt.Errorf("setting host peer %s up: %w", hostVeth, err)
	}
	return nil
}

// ContainerNetworkManager handles interface and routing configuration
// executed inside the container's isolated network namespace.
type ContainerNetworkManager struct {
	nl netlinkOps
}

// NewContainerNetworkManager creates a new ContainerNetworkManager with the provided netlink operations.
func NewContainerNetworkManager(nl netlinkOps) *ContainerNetworkManager {
	return &ContainerNetworkManager{nl: nl}
}

// ConfigureContainerNetwork executes the complete 5-step network configuration sequence:
// 1. Set loopback ("lo") UP.
// 2. Rename contVeth to "eth0".
// 3. Assign ipCIDR to "eth0".
// 4. Set "eth0" UP.
// 5. Add default gateway route via gatewayIP.
func (c *ContainerNetworkManager) ConfigureContainerNetwork(contVeth, ipCIDR, gatewayIP string) error {
	// 1. Bring loopback up
	if err := c.nl.SetLinkUp(LoopbackInterface); err != nil {
		return fmt.Errorf("setting %s up: %w", LoopbackInterface, err)
	}

	// 2. Rename moved veth peer to standard eth0 (if not already eth0)
	targetInterface := ContainerInterface
	if contVeth != targetInterface {
		if err := c.nl.RenameLink(contVeth, targetInterface); err != nil {
			return fmt.Errorf("renaming %s to %s: %w", contVeth, targetInterface, err)
		}
	}

	// 3. Assign IP to eth0
	if err := c.nl.AddAddress(targetInterface, ipCIDR); err != nil {
		return fmt.Errorf("assigning IP %s to %s: %w", ipCIDR, targetInterface, err)
	}

	// 4. Bring eth0 up
	if err := c.nl.SetLinkUp(targetInterface); err != nil {
		return fmt.Errorf("setting %s up: %w", targetInterface, err)
	}

	// 5. Add default route
	if gatewayIP != "" {
		if err := c.nl.AddDefaultRoute(gatewayIP, targetInterface); err != nil {
			return fmt.Errorf("adding default route via %s dev %s: %w", gatewayIP, targetInterface, err)
		}
	}

	return nil
}

// ipForwardPath is the sysctl file controlling IPv4 packet forwarding in the Linux kernel.
const ipForwardPath = "/proc/sys/net/ipv4/ip_forward"

func enableIPForwarding(fs fsWriter) error {
	return fs.WriteFile(ipForwardPath, []byte("1\n"), 0644)
}

// NATManager configures host IP forwarding and iptables MASQUERADE/FORWARD rules.
type NATManager struct {
	fs fsWriter
	ip iptablesRunner
}

func NewNATManager(fs fsWriter, ip iptablesRunner) *NATManager {
	return &NATManager{fs: fs, ip: ip}
}

// EnableOutboundAccess ensures ip_forward is enabled and sets up NAT and forwarding rules.
func (n *NATManager) EnableOutboundAccess(subnetCIDR, bridgeName string) error {
	if err := enableIPForwarding(n.fs); err != nil {
		return fmt.Errorf("enabling ip_forward: %w", err)
	}

	rules := []struct {
		check []string
		add   []string
	}{
		{
			check: []string{"-t", "nat", "-C", "POSTROUTING", "-s", subnetCIDR, "!", "-o", bridgeName, "-j", "MASQUERADE"},
			add:   []string{"-t", "nat", "-A", "POSTROUTING", "-s", subnetCIDR, "!", "-o", bridgeName, "-j", "MASQUERADE"},
		},
		{
			check: []string{"-C", "FORWARD", "-o", bridgeName, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
			add:   []string{"-A", "FORWARD", "-o", bridgeName, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
		},
		{
			check: []string{"-C", "FORWARD", "-i", bridgeName, "!", "-o", bridgeName, "-j", "ACCEPT"},
			add:   []string{"-A", "FORWARD", "-i", bridgeName, "!", "-o", bridgeName, "-j", "ACCEPT"},
		},
		{
			check: []string{"-C", "FORWARD", "-i", bridgeName, "-o", bridgeName, "-j", "ACCEPT"},
			add:   []string{"-A", "FORWARD", "-i", bridgeName, "-o", bridgeName, "-j", "ACCEPT"},
		},
	}

	for _, r := range rules {
		exists, err := n.ip.CheckRule(r.check...)
		if err != nil {
			return fmt.Errorf("checking iptables rule %v: %w", r.check, err)
		}
		if !exists {
			if err := n.ip.RunCommand(r.add...); err != nil {
				return fmt.Errorf("adding iptables rule %v: %w", r.add, err)
			}
		}
	}

	return nil
}

// realNetworkManager is the production implementation of networkManager.
// It composes BridgeManager, VethManager, ContainerNetworkManager, and NATManager.
type realNetworkManager struct {
	bm  *BridgeManager
	vm  *VethManager
	cm  *ContainerNetworkManager
	nat *NATManager
}

// newNetworkManager creates a new realNetworkManager wrapping netlinkOps.
func newNetworkManager(nl netlinkOps) networkManager {
	return &realNetworkManager{
		bm:  NewBridgeManager(nl),
		vm:  NewVethManager(nl),
		cm:  NewContainerNetworkManager(nl),
		nat: NewNATManager(realFsWriter{}, realIptablesRunner{}),
	}
}

func (r *realNetworkManager) EnsureBridge(bridgeName, ipCIDR string) error {
	return r.bm.EnsureBridge(bridgeName, ipCIDR)
}

func (r *realNetworkManager) CreateVethPair(hostVeth, contVeth string) error {
	return r.vm.CreateVethPair(hostVeth, contVeth)
}

func (r *realNetworkManager) AttachToBridge(bridgeName, ifName string) error {
	return r.vm.SetupHostPeer(ifName, bridgeName, r.bm)
}

func (r *realNetworkManager) MoveInterfaceToNetns(ifName string, pid int) error {
	return r.vm.MoveToNetns(ifName, pid)
}

func (r *realNetworkManager) ConfigureContainerNetwork(contVeth, ipCIDR, gatewayIP string) error {
	return r.cm.ConfigureContainerNetwork(contVeth, ipCIDR, gatewayIP)
}

func (r *realNetworkManager) EnableOutboundAccess(subnetCIDR, bridgeName string) error {
	return r.nat.EnableOutboundAccess(subnetCIDR, bridgeName)
}




