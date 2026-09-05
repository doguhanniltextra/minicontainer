package container

import (
	"errors"
	"strings"
	"testing"
)

// mockNetlinkOps tracks netlink operations for testing without root.
type mockNetlinkOps struct {
	existsMap       map[string]bool
	createdBridges  []string
	assignedAddrs   map[string][]string // ifName -> []ipCIDR
	upLinks         []string
	downLinks       []string
	masterMap       map[string]string // ifName -> masterName
	deletedLinks    []string
	vethPairs       [][2]string // [hostVeth, contVeth]
	netnsMap        map[string]int // ifName -> pid
	renamedLinks    map[string]string // oldName -> newName
	defaultRoutes   [][2]string // [gatewayIP, dev]
	opLog           []string // sequence of operations

	existsErr        error
	createBridgeErr  error
	addAddressErr    error
	setLinkUpErr     error
	setLinkDownErr   error
	setLinkMasterErr error
	deleteLinkErr    error
	createVethErr    error
	setNetnsErr      error
	renameErr        error
	addRouteErr      error
	setUpFailOn      string // fail SetLinkUp only on specific interface
}

func newMockNetlinkOps() *mockNetlinkOps {
	return &mockNetlinkOps{
		existsMap:     make(map[string]bool),
		assignedAddrs: make(map[string][]string),
		masterMap:     make(map[string]string),
		netnsMap:      make(map[string]int),
		renamedLinks:  make(map[string]string),
	}
}

func (m *mockNetlinkOps) LinkExists(name string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	return m.existsMap[name], nil
}

func (m *mockNetlinkOps) CreateBridge(name string) error {
	if m.createBridgeErr != nil {
		return m.createBridgeErr
	}
	m.createdBridges = append(m.createdBridges, name)
	m.existsMap[name] = true
	return nil
}

func (m *mockNetlinkOps) AddAddress(ifName string, ipCIDR string) error {
	if m.addAddressErr != nil {
		return m.addAddressErr
	}
	m.assignedAddrs[ifName] = append(m.assignedAddrs[ifName], ipCIDR)
	m.opLog = append(m.opLog, "addr:"+ifName+"="+ipCIDR)
	return nil
}

func (m *mockNetlinkOps) SetLinkUp(ifName string) error {
	if m.setLinkUpErr != nil {
		return m.setLinkUpErr
	}
	if m.setUpFailOn == ifName {
		return errors.New("failed to bring " + ifName + " up")
	}
	m.upLinks = append(m.upLinks, ifName)
	m.opLog = append(m.opLog, "up:"+ifName)
	return nil
}

func (m *mockNetlinkOps) SetLinkDown(ifName string) error {
	if m.setLinkDownErr != nil {
		return m.setLinkDownErr
	}
	m.downLinks = append(m.downLinks, ifName)
	return nil
}

func (m *mockNetlinkOps) SetLinkMaster(ifName, masterName string) error {
	if m.setLinkMasterErr != nil {
		return m.setLinkMasterErr
	}
	m.masterMap[ifName] = masterName
	return nil
}

func (m *mockNetlinkOps) DeleteLink(name string) error {
	if m.deleteLinkErr != nil {
		return m.deleteLinkErr
	}
	m.deletedLinks = append(m.deletedLinks, name)
	delete(m.existsMap, name)
	return nil
}

func (m *mockNetlinkOps) CreateVethPair(hostVeth, contVeth string) error {
	if m.createVethErr != nil {
		return m.createVethErr
	}
	m.vethPairs = append(m.vethPairs, [2]string{hostVeth, contVeth})
	m.existsMap[hostVeth] = true
	m.existsMap[contVeth] = true
	return nil
}

func (m *mockNetlinkOps) SetLinkNetnsByPID(ifName string, pid int) error {
	if m.setNetnsErr != nil {
		return m.setNetnsErr
	}
	m.netnsMap[ifName] = pid
	return nil
}

func (m *mockNetlinkOps) RenameLink(oldName, newName string) error {
	if m.renameErr != nil {
		return m.renameErr
	}
	m.renamedLinks[oldName] = newName
	m.opLog = append(m.opLog, "rename:"+oldName+"->"+newName)
	return nil
}

func (m *mockNetlinkOps) AddDefaultRoute(gatewayIP, ifName string) error {
	if m.addRouteErr != nil {
		return m.addRouteErr
	}
	m.defaultRoutes = append(m.defaultRoutes, [2]string{gatewayIP, ifName})
	m.opLog = append(m.opLog, "route:"+gatewayIP+" via "+ifName)
	return nil
}

func TestEnsureBridge_CreatesWhenNotExisting(t *testing.T) {
	mock := newMockNetlinkOps()
	bm := NewBridgeManager(mock)

	err := bm.EnsureBridge("mc-br0", "172.19.0.1/16")
	if err != nil {
		t.Fatalf("expected EnsureBridge to succeed, got error: %v", err)
	}

	if len(mock.createdBridges) != 1 || mock.createdBridges[0] != "mc-br0" {
		t.Errorf("expected bridge mc-br0 to be created, got %v", mock.createdBridges)
	}

	addrs := mock.assignedAddrs["mc-br0"]
	if len(addrs) != 1 || addrs[0] != "172.19.0.1/16" {
		t.Errorf("expected IP 172.19.0.1/16 to be assigned to mc-br0, got %v", addrs)
	}

	if len(mock.upLinks) != 1 || mock.upLinks[0] != "mc-br0" {
		t.Errorf("expected mc-br0 to be brought up, got %v", mock.upLinks)
	}
}

func TestEnsureBridge_AlreadyExists(t *testing.T) {
	mock := newMockNetlinkOps()
	mock.existsMap["mc-br0"] = true
	bm := NewBridgeManager(mock)

	err := bm.EnsureBridge("mc-br0", "172.19.0.1/16")
	if err != nil {
		t.Fatalf("expected EnsureBridge to succeed when already exists, got: %v", err)
	}

	if len(mock.createdBridges) != 0 {
		t.Errorf("expected no bridge creation when already exists, got: %v", mock.createdBridges)
	}

	if len(mock.assignedAddrs["mc-br0"]) != 0 {
		t.Errorf("expected no address assignment when already exists, got: %v", mock.assignedAddrs["mc-br0"])
	}

	if len(mock.upLinks) != 1 || mock.upLinks[0] != "mc-br0" {
		t.Errorf("expected mc-br0 to still be ensured UP, got: %v", mock.upLinks)
	}
}

func TestEnsureBridge_ErrorsOnCreateFailure(t *testing.T) {
	mock := newMockNetlinkOps()
	mock.createBridgeErr = errors.New("permission denied")
	bm := NewBridgeManager(mock)

	err := bm.EnsureBridge("mc-br0", "172.19.0.1/16")
	if err == nil {
		t.Fatal("expected error on bridge creation failure, got nil")
	}
}

func TestEnsureBridge_ErrorsOnAddressFailure(t *testing.T) {
	mock := newMockNetlinkOps()
	mock.addAddressErr = errors.New("address conflict")
	bm := NewBridgeManager(mock)

	err := bm.EnsureBridge("mc-br0", "172.19.0.1/16")
	if err == nil {
		t.Fatal("expected error on address assignment failure, got nil")
	}
}

func TestEnsureBridge_ErrorsOnSetUpFailure(t *testing.T) {
	mock := newMockNetlinkOps()
	mock.setLinkUpErr = errors.New("link failed to go up")
	bm := NewBridgeManager(mock)

	err := bm.EnsureBridge("mc-br0", "172.19.0.1/16")
	if err == nil {
		t.Fatal("expected error on set link up failure, got nil")
	}
}

func TestAttachToBridge_SetsLinkMaster(t *testing.T) {
	mock := newMockNetlinkOps()
	bm := NewBridgeManager(mock)

	err := bm.AttachToBridge("mc-br0", "veth-host-1234")
	if err != nil {
		t.Fatalf("expected AttachToBridge to succeed, got error: %v", err)
	}

	if mock.masterMap["veth-host-1234"] != "mc-br0" {
		t.Errorf("expected veth-host-1234 master to be mc-br0, got %q", mock.masterMap["veth-host-1234"])
	}
}

func TestAttachToBridge_ErrorsOnFailure(t *testing.T) {
	mock := newMockNetlinkOps()
	mock.setLinkMasterErr = errors.New("device busy")
	bm := NewBridgeManager(mock)

	err := bm.AttachToBridge("mc-br0", "veth-host-1234")
	if err == nil {
		t.Fatal("expected error when setting link master fails, got nil")
	}
}

func TestGenerateVethNames_ValidFormat(t *testing.T) {
	host1, cont1, err := GenerateVethNames()
	if err != nil {
		t.Fatalf("GenerateVethNames failed: %v", err)
	}

	if !strings.HasPrefix(host1, "mc-h-") {
		t.Errorf("expected host veth to have prefix mc-h-, got %s", host1)
	}
	if !strings.HasPrefix(cont1, "mc-c-") {
		t.Errorf("expected cont veth to have prefix mc-c-, got %s", cont1)
	}

	// Linux IFNAMSIZ limit is 16 bytes including null terminator -> max 15 characters
	if len(host1) > 15 {
		t.Errorf("host veth name %s exceeds 15 characters (%d)", host1, len(host1))
	}
	if len(cont1) > 15 {
		t.Errorf("cont veth name %s exceeds 15 characters (%d)", cont1, len(cont1))
	}

	// Suffixes must match
	if host1[5:] != cont1[5:] {
		t.Errorf("expected matching suffix between %s and %s", host1, cont1)
	}

	// Consecutive calls must generate unique names
	host2, _, _ := GenerateVethNames()
	if host1 == host2 {
		t.Errorf("expected distinct names between calls, got %s and %s", host1, host2)
	}
}

func TestCreateVethPair_CallsNetlink(t *testing.T) {
	mock := newMockNetlinkOps()
	vm := NewVethManager(mock)

	err := vm.CreateVethPair("mc-h-123", "mc-c-123")
	if err != nil {
		t.Fatalf("expected CreateVethPair to succeed, got: %v", err)
	}

	if len(mock.vethPairs) != 1 {
		t.Fatalf("expected 1 veth pair created, got %d", len(mock.vethPairs))
	}
	if mock.vethPairs[0] != [2]string{"mc-h-123", "mc-c-123"} {
		t.Errorf("unexpected veth pair: %v", mock.vethPairs[0])
	}
}

func TestCreateVethPair_ErrorPropagation(t *testing.T) {
	mock := newMockNetlinkOps()
	mock.createVethErr = errors.New("cannot create veth")
	vm := NewVethManager(mock)

	err := vm.CreateVethPair("mc-h-123", "mc-c-123")
	if err == nil {
		t.Fatal("expected error on veth creation failure, got nil")
	}
}

func TestMoveToNetns_CallsSetLinkNetnsByPID(t *testing.T) {
	mock := newMockNetlinkOps()
	vm := NewVethManager(mock)

	err := vm.MoveToNetns("mc-c-123", 4567)
	if err != nil {
		t.Fatalf("expected MoveToNetns to succeed, got: %v", err)
	}

	if mock.netnsMap["mc-c-123"] != 4567 {
		t.Errorf("expected mc-c-123 to be moved to pid 4567, got %d", mock.netnsMap["mc-c-123"])
	}
}

func TestMoveToNetns_ErrorPropagation(t *testing.T) {
	mock := newMockNetlinkOps()
	mock.setNetnsErr = errors.New("no such process")
	vm := NewVethManager(mock)

	err := vm.MoveToNetns("mc-c-123", 4567)
	if err == nil {
		t.Fatal("expected error on netns migration failure, got nil")
	}
}

func TestSetupHostPeer_AttachesAndSetsUp(t *testing.T) {
	mock := newMockNetlinkOps()
	bm := NewBridgeManager(mock)
	vm := NewVethManager(mock)

	err := vm.SetupHostPeer("mc-h-123", "mc-br0", bm)
	if err != nil {
		t.Fatalf("expected SetupHostPeer to succeed, got: %v", err)
	}

	if mock.masterMap["mc-h-123"] != "mc-br0" {
		t.Errorf("expected mc-h-123 master to be mc-br0, got %s", mock.masterMap["mc-h-123"])
	}

	foundUp := false
	for _, ifName := range mock.upLinks {
		if ifName == "mc-h-123" {
			foundUp = true
			break
		}
	}
	if !foundUp {
		t.Errorf("expected mc-h-123 to be brought UP, got upLinks: %v", mock.upLinks)
	}
}

func TestSetupHostPeer_AttachError(t *testing.T) {
	mock := newMockNetlinkOps()
	mock.setLinkMasterErr = errors.New("attach failed")
	bm := NewBridgeManager(mock)
	vm := NewVethManager(mock)

	err := vm.SetupHostPeer("mc-h-123", "mc-br0", bm)
	if err == nil {
		t.Fatal("expected error when attaching host peer fails, got nil")
	}
}

func TestSetupHostPeer_SetUpError(t *testing.T) {
	mock := newMockNetlinkOps()
	mock.setLinkUpErr = errors.New("link up failed")
	bm := NewBridgeManager(mock)
	vm := NewVethManager(mock)

	err := vm.SetupHostPeer("mc-h-123", "mc-br0", bm)
	if err == nil {
		t.Fatal("expected error when bringing host peer up fails, got nil")
	}
}

func TestConfigureContainerNetwork_CompleteOrder(t *testing.T) {
	mock := newMockNetlinkOps()
	cm := NewContainerNetworkManager(mock)

	err := cm.ConfigureContainerNetwork("mc-c-1234", "172.19.0.2/16", "172.19.0.1")
	if err != nil {
		t.Fatalf("expected ConfigureContainerNetwork to succeed, got: %v", err)
	}

	expectedLog := []string{
		"up:lo",
		"rename:mc-c-1234->eth0",
		"addr:eth0=172.19.0.2/16",
		"up:eth0",
		"route:172.19.0.1 via eth0",
	}

	if len(mock.opLog) != len(expectedLog) {
		t.Fatalf("expected %d operations, got %d: %v", len(expectedLog), len(mock.opLog), mock.opLog)
	}

	for i, op := range expectedLog {
		if mock.opLog[i] != op {
			t.Errorf("step %d: expected %q, got %q", i, op, mock.opLog[i])
		}
	}
}

func TestConfigureContainerNetwork_LoopbackError(t *testing.T) {
	mock := newMockNetlinkOps()
	mock.setLinkUpErr = errors.New("lo down")
	cm := NewContainerNetworkManager(mock)

	err := cm.ConfigureContainerNetwork("mc-c-1234", "172.19.0.2/16", "172.19.0.1")
	if err == nil {
		t.Fatal("expected error on loopback bringup failure, got nil")
	}
}

func TestConfigureContainerNetwork_RenameError(t *testing.T) {
	mock := newMockNetlinkOps()
	mock.renameErr = errors.New("name exists")
	cm := NewContainerNetworkManager(mock)

	err := cm.ConfigureContainerNetwork("mc-c-1234", "172.19.0.2/16", "172.19.0.1")
	if err == nil {
		t.Fatal("expected error on link rename failure, got nil")
	}
}

func TestConfigureContainerNetwork_AddressError(t *testing.T) {
	mock := newMockNetlinkOps()
	mock.addAddressErr = errors.New("ip invalid")
	cm := NewContainerNetworkManager(mock)

	err := cm.ConfigureContainerNetwork("mc-c-1234", "172.19.0.2/16", "172.19.0.1")
	if err == nil {
		t.Fatal("expected error on address assignment failure, got nil")
	}
}

func TestConfigureContainerNetwork_SetUpError(t *testing.T) {
	mock := newMockNetlinkOps()
	mock.setUpFailOn = "eth0"
	cm := NewContainerNetworkManager(mock)

	err := cm.ConfigureContainerNetwork("mc-c-1234", "172.19.0.2/16", "172.19.0.1")
	if err == nil {
		t.Fatal("expected error on eth0 bringup failure, got nil")
	}
}

func TestConfigureContainerNetwork_RouteError(t *testing.T) {
	mock := newMockNetlinkOps()
	mock.addRouteErr = errors.New("network unreachable")
	cm := NewContainerNetworkManager(mock)

	err := cm.ConfigureContainerNetwork("mc-c-1234", "172.19.0.2/16", "172.19.0.1")
	if err == nil {
		t.Fatal("expected error on default route installation failure, got nil")
	}
}

type mockIptablesRunner struct {
	checkedRules  [][]string
	ranCommands   [][]string
	existingRules map[string]bool
	checkErr      error
	runErr        error
}

func newMockIptablesRunner() *mockIptablesRunner {
	return &mockIptablesRunner{
		existingRules: make(map[string]bool),
	}
}

func (m *mockIptablesRunner) CheckRule(args ...string) (bool, error) {
	if m.checkErr != nil {
		return false, m.checkErr
	}
	m.checkedRules = append(m.checkedRules, args)
	key := strings.Join(args, " ")
	return m.existingRules[key], nil
}

func (m *mockIptablesRunner) RunCommand(args ...string) error {
	if m.runErr != nil {
		return m.runErr
	}
	m.ranCommands = append(m.ranCommands, args)
	return nil
}

func TestEnableIPForwarding_WritesOneToFile(t *testing.T) {
	mockFs := newMockFsWriter()
	if err := enableIPForwarding(mockFs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, ok := mockFs.writeFileCalls[ipForwardPath]
	if !ok {
		t.Fatalf("expected write to %s", ipForwardPath)
	}
	if content != "1\n" {
		t.Errorf("expected %q, got %q", "1\n", content)
	}
}

func TestEnableIPForwarding_ErrorPropagation(t *testing.T) {
	mockFs := newMockFsWriter()
	mockFs.writeFileErr = errors.New("permission denied")

	if err := enableIPForwarding(mockFs); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNATManager_AddsMissingRules(t *testing.T) {
	mockFs := newMockFsWriter()
	mockIp := newMockIptablesRunner()
	nat := NewNATManager(mockFs, mockIp)

	err := nat.EnableOutboundAccess("172.19.0.0/16", "mc-br0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mockFs.writeFileCalls[ipForwardPath] != "1\n" {
		t.Errorf("expected ip_forward to be 1, got %q", mockFs.writeFileCalls[ipForwardPath])
	}

	// 4 rules should be checked and added
	if len(mockIp.checkedRules) != 4 {
		t.Fatalf("expected 4 checked rules, got %d", len(mockIp.checkedRules))
	}
	if len(mockIp.ranCommands) != 4 {
		t.Fatalf("expected 4 ran commands, got %d", len(mockIp.ranCommands))
	}

	// Verify MASQUERADE command
	expectedMasq := []string{"-t", "nat", "-A", "POSTROUTING", "-s", "172.19.0.0/16", "!", "-o", "mc-br0", "-j", "MASQUERADE"}
	if strings.Join(mockIp.ranCommands[0], " ") != strings.Join(expectedMasq, " ") {
		t.Errorf("expected %v, got %v", expectedMasq, mockIp.ranCommands[0])
	}

	// Verify FORWARD related,established command
	expectedRelated := []string{"-A", "FORWARD", "-o", "mc-br0", "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}
	if strings.Join(mockIp.ranCommands[1], " ") != strings.Join(expectedRelated, " ") {
		t.Errorf("expected %v, got %v", expectedRelated, mockIp.ranCommands[1])
	}

	// Verify FORWARD out command
	expectedOut := []string{"-A", "FORWARD", "-i", "mc-br0", "!", "-o", "mc-br0", "-j", "ACCEPT"}
	if strings.Join(mockIp.ranCommands[2], " ") != strings.Join(expectedOut, " ") {
		t.Errorf("expected %v, got %v", expectedOut, mockIp.ranCommands[2])
	}

	// Verify FORWARD bridge-to-bridge command
	expectedBridge := []string{"-A", "FORWARD", "-i", "mc-br0", "-o", "mc-br0", "-j", "ACCEPT"}
	if strings.Join(mockIp.ranCommands[3], " ") != strings.Join(expectedBridge, " ") {
		t.Errorf("expected %v, got %v", expectedBridge, mockIp.ranCommands[3])
	}
}

func TestNATManager_IdempotentRules(t *testing.T) {
	mockFs := newMockFsWriter()
	mockIp := newMockIptablesRunner()

	// Pre-populate existing rules
	mockIp.existingRules["-t nat -C POSTROUTING -s 172.19.0.0/16 ! -o mc-br0 -j MASQUERADE"] = true
	mockIp.existingRules["-C FORWARD -o mc-br0 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT"] = true
	mockIp.existingRules["-C FORWARD -i mc-br0 ! -o mc-br0 -j ACCEPT"] = true
	mockIp.existingRules["-C FORWARD -i mc-br0 -o mc-br0 -j ACCEPT"] = true

	nat := NewNATManager(mockFs, mockIp)

	err := nat.EnableOutboundAccess("172.19.0.0/16", "mc-br0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mockIp.ranCommands) != 0 {
		t.Errorf("expected 0 commands executed when rules exist, got %d: %v", len(mockIp.ranCommands), mockIp.ranCommands)
	}
}

func TestNATManager_ErrorPropagation(t *testing.T) {
	t.Run("ip_forward error", func(t *testing.T) {
		mockFs := newMockFsWriter()
		mockFs.writeFileErr = errors.New("write error")
		mockIp := newMockIptablesRunner()
		nat := NewNATManager(mockFs, mockIp)

		if err := nat.EnableOutboundAccess("172.19.0.0/16", "mc-br0"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("check rule error", func(t *testing.T) {
		mockFs := newMockFsWriter()
		mockIp := newMockIptablesRunner()
		mockIp.checkErr = errors.New("iptables check error")
		nat := NewNATManager(mockFs, mockIp)

		if err := nat.EnableOutboundAccess("172.19.0.0/16", "mc-br0"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("run command error", func(t *testing.T) {
		mockFs := newMockFsWriter()
		mockIp := newMockIptablesRunner()
		mockIp.runErr = errors.New("iptables add error")
		nat := NewNATManager(mockFs, mockIp)

		if err := nat.EnableOutboundAccess("172.19.0.0/16", "mc-br0"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}



