package container

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
)

// mockCmdRunner records commands and controls process execution simulation.
type mockCmdRunner struct {
	lastCmd  *exec.Cmd
	runErr   error
	startErr error
	waitErr  error

	onStart func()
	onWait  func()
}

func (m *mockCmdRunner) Run(cmd *exec.Cmd) error {
	m.lastCmd = cmd
	return m.runErr
}

func (m *mockCmdRunner) Start(cmd *exec.Cmd) error {
	m.lastCmd = cmd
	if cmd.Process == nil {
		cmd.Process = &os.Process{Pid: 12345}
	}
	if m.onStart != nil {
		m.onStart()
	}
	if m.startErr != nil {
		return m.startErr
	}
	return m.runErr
}

func (m *mockCmdRunner) Wait(cmd *exec.Cmd) error {
	if m.onWait != nil {
		m.onWait()
	}
	return m.waitErr
}

// mockCgroupManager records cgroup lifecycle calls and parameters for unit testing.
type mockCgroupManager struct {
	calls         []string
	appliedCfg    Config
	addedPid      int
	cleanedUp     bool
	applyErr      error
	addProcessErr error
	cleanupErr    error

	onApply      func()
	onAddProcess func()
	onCleanup    func()
}

func (m *mockCgroupManager) apply(cfg Config) error {
	m.calls = append(m.calls, "apply")
	m.appliedCfg = cfg
	if m.onApply != nil {
		m.onApply()
	}
	return m.applyErr
}

func (m *mockCgroupManager) addProcess(pid int) error {
	m.calls = append(m.calls, "addProcess")
	m.addedPid = pid
	if m.onAddProcess != nil {
		m.onAddProcess()
	}
	return m.addProcessErr
}

func (m *mockCgroupManager) cleanup() error {
	m.calls = append(m.calls, "cleanup")
	m.cleanedUp = true
	if m.onCleanup != nil {
		m.onCleanup()
	}
	return m.cleanupErr
}

// mockExecer records the arguments passed to Exec and can return a configured error.
type mockExecer struct {
	calledArgv0 string
	calledArgv  []string
	err         error
}

func (m *mockExecer) Exec(argv0 string, argv []string, envv []string) error {
	m.calledArgv0 = argv0
	m.calledArgv = argv
	return m.err
}

// mockNetworkManager records network lifecycle calls for testing container.go.
type mockNetworkManager struct {
	events          []string
	createdHostVeth string
	createdContVeth string
	netnsPid        int
	attachedBridge  string
	attachedIf      string

	ensureBridgeErr   error
	enableOutboundErr error
	createVethErr     error
	attachBridgeErr   error
	moveToNetnsErr    error
	configureNetErr   error

	onEnsureBridge   func()
	onEnableOutbound func()
	onCreateVeth     func()
	onAttachBridge   func()
	onMoveToNetns    func()
	onConfigureNet   func()
}

func (m *mockNetworkManager) EnsureBridge(bridgeName, ipCIDR string) error {
	m.events = append(m.events, "ensureBridge")
	if m.onEnsureBridge != nil {
		m.onEnsureBridge()
	}
	return m.ensureBridgeErr
}

func (m *mockNetworkManager) EnableOutboundAccess(subnetCIDR, bridgeName string) error {
	m.events = append(m.events, "enableOutboundAccess")
	if m.onEnableOutbound != nil {
		m.onEnableOutbound()
	}
	return m.enableOutboundErr
}

func (m *mockNetworkManager) CreateVethPair(hostVeth, contVeth string) error {
	m.events = append(m.events, "createVeth")
	m.createdHostVeth = hostVeth
	m.createdContVeth = contVeth
	if m.onCreateVeth != nil {
		m.onCreateVeth()
	}
	return m.createVethErr
}

func (m *mockNetworkManager) AttachToBridge(bridgeName, ifName string) error {
	m.events = append(m.events, "attachBridge")
	m.attachedBridge = bridgeName
	m.attachedIf = ifName
	if m.onAttachBridge != nil {
		m.onAttachBridge()
	}
	return m.attachBridgeErr
}

func (m *mockNetworkManager) MoveInterfaceToNetns(ifName string, pid int) error {
	m.events = append(m.events, "moveToNetns")
	m.netnsPid = pid
	if m.onMoveToNetns != nil {
		m.onMoveToNetns()
	}
	return m.moveToNetnsErr
}

func (m *mockNetworkManager) ConfigureContainerNetwork(contVeth, ipCIDR, gatewayIP string) error {
	m.events = append(m.events, "configureNet")
	if m.onConfigureNet != nil {
		m.onConfigureNet()
	}
	return m.configureNetErr
}

// mockSyncWaiter records Wait() calls for testing container-init synchronization.
type mockSyncWaiter struct {
	waited  bool
	waitErr error
	onWait  func()
}

func (m *mockSyncWaiter) Wait() error {
	m.waited = true
	if m.onWait != nil {
		m.onWait()
	}
	return m.waitErr
}

// --- runWith tests ---

func TestRunWith_ReexecsSelfWithContainerInitArg(t *testing.T) {
	mock := &mockCmdRunner{}

	cfg := Config{Command: "/bin/sh", Hostname: "test-box", Rootfs: "assets/rootfs"}
	if err := runWith(cfg, mock); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd := mock.lastCmd
	if cmd == nil {
		t.Fatal("expected command to be started, got nil")
	}
	// Must re-exec the current binary, not /bin/sh directly
	if cmd.Path != "/proc/self/exe" {
		t.Errorf("expected path /proc/self/exe, got %q", cmd.Path)
	}
	// First argument must be the hidden container-init sentinel
	if len(cmd.Args) < 2 || cmd.Args[1] != ContainerInitArg {
		t.Errorf("expected args[1] to be %q, got %v", ContainerInitArg, cmd.Args)
	}
	// Second argument must be the requested command
	if len(cmd.Args) < 3 || cmd.Args[2] != "/bin/sh" {
		t.Errorf("expected args[2] to be /bin/sh, got %v", cmd.Args)
	}
}

func TestRunWith_PassesHostnameAndRootfsViaEnv(t *testing.T) {
	mock := &mockCmdRunner{}

	cfg := Config{Command: "/bin/sh", Hostname: "my-container", Rootfs: "/var/rootfs"}
	if err := runWith(cfg, mock); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedHost := hostnameEnvKey + "=my-container"
	expectedRootfs := rootfsEnvKey + "=/var/rootfs"

	foundHost := false
	foundRootfs := false
	for _, env := range mock.lastCmd.Env {
		if env == expectedHost {
			foundHost = true
		}
		if env == expectedRootfs {
			foundRootfs = true
		}
	}
	if !foundHost {
		t.Errorf("%q not found in cmd.Env: %v", expectedHost, mock.lastCmd.Env)
	}
	if !foundRootfs {
		t.Errorf("%q not found in cmd.Env: %v", expectedRootfs, mock.lastCmd.Env)
	}
}

func TestRunWith_HasNamespaceFlags(t *testing.T) {
	mock := &mockCmdRunner{}

	cfg := Config{Command: "/bin/sh", Hostname: "test-box", Rootfs: "assets/rootfs"}
	if err := runWith(cfg, mock); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	attr := mock.lastCmd.SysProcAttr
	if attr == nil {
		t.Fatal("expected SysProcAttr to be set, got nil")
	}
	for name, flag := range map[string]uintptr{
		"CLONE_NEWPID": syscall.CLONE_NEWPID,
		"CLONE_NEWUTS": syscall.CLONE_NEWUTS,
		"CLONE_NEWNET": syscall.CLONE_NEWNET,
		"CLONE_NEWNS":  syscall.CLONE_NEWNS,
	} {
		if attr.Cloneflags&flag == 0 {
			t.Errorf("missing namespace flag: %s", name)
		}
	}
}

func TestRunWith_PropagatesRunnerError(t *testing.T) {
	expected := errors.New("fork failed")
	mock := &mockCmdRunner{runErr: expected}

	cfg := Config{Command: "/bin/sh", Hostname: "test-box", Rootfs: "assets/rootfs"}
	err := runWith(cfg, mock)
	if !errors.Is(err, expected) {
		t.Errorf("expected error %v, got %v", expected, err)
	}
}

func TestRunWith_CreatesCgroupBeforeStart(t *testing.T) {
	var events []string
	mockCg := &mockCgroupManager{
		onApply: func() {
			events = append(events, "apply")
		},
	}
	mockRunner := &mockCmdRunner{
		onStart: func() {
			events = append(events, "start")
		},
	}

	cfg := Config{
		Command:     "/bin/sh",
		MemoryLimit: 104857600,
	}

	if err := runWith(cfg, mockRunner, mockCg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %v", events)
	}
	if events[0] != "apply" || events[1] != "start" {
		t.Errorf("expected apply before start, got event order: %v", events)
	}
	if mockCg.appliedCfg.MemoryLimit != 104857600 {
		t.Errorf("expected MemoryLimit 104857600, got %d", mockCg.appliedCfg.MemoryLimit)
	}
}

func TestRunWith_AddsPidAfterStart(t *testing.T) {
	var events []string
	mockCg := &mockCgroupManager{
		onAddProcess: func() {
			events = append(events, "addProcess")
		},
	}
	mockRunner := &mockCmdRunner{
		onStart: func() {
			events = append(events, "start")
		},
		onWait: func() {
			events = append(events, "wait")
		},
	}

	cfg := Config{
		Command:   "/bin/sh",
		PidsLimit: 20,
	}

	if err := runWith(cfg, mockRunner, mockCg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %v", events)
	}
	if events[0] != "start" || events[1] != "addProcess" || events[2] != "wait" {
		t.Errorf("expected start -> addProcess -> wait, got: %v", events)
	}
	if mockCg.addedPid != 12345 {
		t.Errorf("expected child PID 12345 to be added to cgroup, got %d", mockCg.addedPid)
	}
}

func TestRunWith_CleansUpOnExit(t *testing.T) {
	mockCg := &mockCgroupManager{}
	mockRunner := &mockCmdRunner{}

	cfg := Config{
		Command:  "/bin/sh",
		CpuQuota: 50000,
	}

	if err := runWith(cfg, mockRunner, mockCg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mockCg.cleanedUp {
		t.Error("expected cgroup cleanup() to be called on exit")
	}
}

func TestRunWith_NoCgroup_WhenNoLimitsSet(t *testing.T) {
	mockCg := &mockCgroupManager{}
	mockRunner := &mockCmdRunner{}

	cfg := Config{
		Command:  "/bin/sh",
		Hostname: "test-box",
		Rootfs:   "assets/rootfs",
	}

	if err := runWith(cfg, mockRunner, mockCg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mockCg.calls) > 0 {
		t.Errorf("expected no cgroup operations when limits are zero, got calls: %v", mockCg.calls)
	}
}

func TestRunWith_CgroupApplyError_AbortsBeforeStart(t *testing.T) {
	applyErr := errors.New("cannot apply limits")
	mockCg := &mockCgroupManager{applyErr: applyErr}
	mockRunner := &mockCmdRunner{}

	cfg := Config{Command: "/bin/sh", MemoryLimit: 104857600}
	err := runWith(cfg, mockRunner, mockCg)

	if !errors.Is(err, applyErr) {
		t.Errorf("expected apply error %v, got %v", applyErr, err)
	}
	if mockRunner.lastCmd != nil {
		t.Error("expected runner.Start not to be called when apply fails")
	}
	if !mockCg.cleanedUp {
		t.Error("expected cleanup to be called even when apply fails")
	}
}

func TestRunWith_CgroupAddProcessError_Aborts(t *testing.T) {
	addErr := errors.New("cannot add pid")
	mockCg := &mockCgroupManager{addProcessErr: addErr}
	mockRunner := &mockCmdRunner{}

	cfg := Config{Command: "/bin/sh", PidsLimit: 20}
	err := runWith(cfg, mockRunner, mockCg)

	if !errors.Is(err, addErr) {
		t.Errorf("expected addProcess error %v, got %v", addErr, err)
	}
	if !mockCg.cleanedUp {
		t.Error("expected cleanup to be called when addProcess fails")
	}
}

// --- initWith tests ---

func TestInitWith_FullLifecycle(t *testing.T) {
	tempRootfs := t.TempDir()
	mockH := &mockHostnamer{}
	mockM := &mockMounter{}
	mockP := &mockPivotRooter{}
	mockE := &mockExecer{}

	args := []string{"/bin/sh", "-c", "echo test"}
	err := initWith("test-host", tempRootfs, "/bin/sh", args, mockH, mockM, mockP, mockE)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1. Hostname must be set
	if mockH.lastHostname != "test-host" {
		t.Errorf("expected hostname %q, got %q", "test-host", mockH.lastHostname)
	}

	// 2. pivotRoot must be called with tempRootfs
	if mockP.lastNewRoot != tempRootfs {
		t.Errorf("expected pivotRoot on %q, got %q", tempRootfs, mockP.lastNewRoot)
	}

	// 3. Exec must be called
	if mockE.calledArgv0 != "/bin/sh" {
		t.Errorf("expected exec argv0 /bin/sh, got %q", mockE.calledArgv0)
	}
}

func TestInitWith_HostnameError_Aborts(t *testing.T) {
	tempRootfs := t.TempDir()
	expectedErr := errors.New("operation not permitted")
	mockH := &mockHostnamer{err: expectedErr}
	mockM := &mockMounter{}
	mockP := &mockPivotRooter{}
	mockE := &mockExecer{}

	err := initWith("bad", tempRootfs, "/bin/sh", []string{"/bin/sh"}, mockH, mockM, mockP, mockE)
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected wrapped error %v, got %v", expectedErr, err)
	}
	if mockP.lastNewRoot != "" {
		t.Errorf("expected no pivotRoot on hostname failure")
	}
}

func TestInitWith_PivotRootError_Aborts(t *testing.T) {
	tempRootfs := t.TempDir()
	expectedErr := errors.New("pivot error")
	mockH := &mockHostnamer{}
	mockM := &mockMounter{}
	mockP := &mockPivotRooter{pivotErr: expectedErr}
	mockE := &mockExecer{}

	err := initWith("test", tempRootfs, "/bin/sh", []string{"/bin/sh"}, mockH, mockM, mockP, mockE)
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected wrapped error %v, got %v", expectedErr, err)
	}
	if mockE.calledArgv0 != "" {
		t.Errorf("expected no exec on pivot failure")
	}
}

func TestInitWith_MountError_AbortsBeforeExec(t *testing.T) {
	tempRootfs := t.TempDir()
	expectedErr := errors.New("mount error")
	mockH := &mockHostnamer{}
	mockM := &mockMounter{mountErr: expectedErr}
	mockP := &mockPivotRooter{}
	mockE := &mockExecer{}

	err := initWith("test", tempRootfs, "/bin/sh", []string{"/bin/sh"}, mockH, mockM, mockP, mockE)
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected wrapped error %v, got %v", expectedErr, err)
	}
	if mockE.calledArgv0 != "" {
		t.Errorf("expected no exec on mount failure, got %q", mockE.calledArgv0)
	}
}

func TestRunWith_NetworkSetupSequence(t *testing.T) {
	var events []string
	mockNet := &mockNetworkManager{
		onEnsureBridge:   func() { events = append(events, "ensureBridge") },
		onEnableOutbound: func() { events = append(events, "enableOutboundAccess") },
		onCreateVeth:     func() { events = append(events, "createVeth") },
		onMoveToNetns:    func() { events = append(events, "moveToNetns") },
		onAttachBridge:   func() { events = append(events, "attachBridge") },
	}
	mockRunner := &mockCmdRunner{
		onStart: func() { events = append(events, "start") },
		onWait:  func() { events = append(events, "wait") },
	}

	cfg := Config{
		Command: "/bin/sh",
	}

	if err := runWith(cfg, mockRunner, mockNet); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedOrder := []string{
		"ensureBridge",
		"enableOutboundAccess",
		"createVeth",
		"start",
		"moveToNetns",
		"attachBridge",
		"wait",
	}

	if len(events) != len(expectedOrder) {
		t.Fatalf("expected %d events, got %d: %v", len(expectedOrder), len(events), events)
	}

	for i, expected := range expectedOrder {
		if events[i] != expected {
			t.Errorf("step %d: expected %s, got %s (full trace: %v)", i, expected, events[i], events)
		}
	}

	// Verify that extra file (sync pipe) was attached to the command
	if len(mockRunner.lastCmd.ExtraFiles) == 0 {
		t.Error("expected sync pipe to be passed in ExtraFiles, got none")
	}

	// Verify child PID was forwarded to MoveInterfaceToNetns
	if mockNet.netnsPid != 12345 {
		t.Errorf("expected child PID 12345 to be passed to MoveInterfaceToNetns, got %d", mockNet.netnsPid)
	}
}

func TestRunWith_ReleasesIPOnExit(t *testing.T) {
	ipam, err := NewIPAM("172.19.0.0/16", "172.19.0.1")
	if err != nil {
		t.Fatalf("NewIPAM failed: %v", err)
	}

	mockNet := &mockNetworkManager{}
	mockRunner := &mockCmdRunner{}

	cfg := Config{Command: "/bin/sh"}

	if err := runWith(cfg, mockRunner, mockNet, ipam); err != nil {
		t.Fatalf("runWith failed: %v", err)
	}

	// Because IP was released on container exit, allocating now should return 172.19.0.2/16 again
	nextIP, err := ipam.Allocate()
	if err != nil {
		t.Fatalf("Allocate after container exit failed: %v", err)
	}
	if nextIP != "172.19.0.2/16" {
		t.Errorf("expected released IP 172.19.0.2/16 to be reused, got %s", nextIP)
	}
}

func TestInitWith_WaitsOnSyncPipeAndConfiguresNetwork(t *testing.T) {
	var events []string
	mockNet := &mockNetworkManager{
		onConfigureNet: func() { events = append(events, "configureNet") },
	}
	mockSyncer := &mockSyncWaiter{
		onWait: func() { events = append(events, "syncWait") },
	}
	mockH := &mockHostnamer{}
	mockM := &mockMounter{}
	mockP := &mockPivotRooter{}
	mockE := &mockExecer{}

	netCfg := networkInitConfig{
		netMgr:   mockNet,
		syncer:   mockSyncer,
		contVeth: "mc-c-test",
		ip:       "172.19.0.2/16",
		gateway:  "172.19.0.1",
	}

	tempRootfs := t.TempDir()
	err := initWith("test-host", tempRootfs, "/bin/sh", []string{"/bin/sh"}, mockH, mockM, mockP, mockE, netCfg)
	if err != nil {
		t.Fatalf("initWith failed: %v", err)
	}

	if !mockSyncer.waited {
		t.Error("expected syncer.Wait() to be called")
	}

	expectedOrder := []string{"syncWait", "configureNet"}
	if len(events) != len(expectedOrder) {
		t.Fatalf("expected %d events, got %d: %v", len(expectedOrder), len(events), events)
	}
	for i, expected := range expectedOrder {
		if events[i] != expected {
			t.Errorf("step %d: expected %s, got %s", i, expected, events[i])
		}
	}
}

// mockContainerStore records container store lifecycle operations.
type mockContainerStore struct {
	createdRoots  []string
	createdID     string
	createdMerged string
	createErr     error

	destroyedIDs []string
	destroyErr   error

	upperPath  string
	workPath   string
	mergedPath string

	onCreate  func()
	onDestroy func()
}

func (m *mockContainerStore) Create(imageRootfs string) (string, string, error) {
	m.createdRoots = append(m.createdRoots, imageRootfs)
	if m.onCreate != nil {
		m.onCreate()
	}
	if m.createErr != nil {
		return "", "", m.createErr
	}
	id := m.createdID
	if id == "" {
		id = "mockcont1234"
	}
	merged := m.createdMerged
	if merged == "" {
		merged = "/containers/" + id + "/merged"
	}
	return id, merged, nil
}

func (m *mockContainerStore) Destroy(id string) error {
	m.destroyedIDs = append(m.destroyedIDs, id)
	if m.onDestroy != nil {
		m.onDestroy()
	}
	return m.destroyErr
}

func (m *mockContainerStore) MergedPath(id string) string {
	if m.mergedPath != "" {
		return m.mergedPath
	}
	return "/containers/" + id + "/merged"
}

func (m *mockContainerStore) UpperPath(id string) string {
	if m.upperPath != "" {
		return m.upperPath
	}
	return "/containers/" + id + "/upper"
}

func (m *mockContainerStore) WorkPath(id string) string {
	if m.workPath != "" {
		return m.workPath
	}
	return "/containers/" + id + "/work"
}

// mockOverlayMounter records overlay mount and unmount calls.
type mockOverlayMounter struct {
	mountCalls   [][]string
	unmountCalls []string
	mountErr     error
	unmountErr   error

	onMount   func()
	onUnmount func()
}

func (m *mockOverlayMounter) Mount(lowerdir, upperdir, workdir, mergeddir string) error {
	m.mountCalls = append(m.mountCalls, []string{lowerdir, upperdir, workdir, mergeddir})
	if m.onMount != nil {
		m.onMount()
	}
	return m.mountErr
}

func (m *mockOverlayMounter) Unmount(mergeddir string) error {
	m.unmountCalls = append(m.unmountCalls, mergeddir)
	if m.onUnmount != nil {
		m.onUnmount()
	}
	return m.unmountErr
}

func TestRunWith_CreatesOverlayBeforeStart(t *testing.T) {
	var order []string

	mockRunner := &mockCmdRunner{
		onStart: func() { order = append(order, "runner.Start") },
	}
	mockStore := &mockContainerStore{
		onCreate: func() { order = append(order, "store.Create") },
	}
	mockOverlay := &mockOverlayMounter{
		onMount: func() { order = append(order, "overlay.Mount") },
	}

	cfg := Config{
		Command:  "/bin/sh",
		Hostname: "cow-box",
		Rootfs:   "/images/alpine/rootfs",
	}

	err := runWith(cfg, mockRunner, mockStore, mockOverlay)
	if err != nil {
		t.Fatalf("runWith failed: %v", err)
	}

	// Verify order: store.Create -> overlay.Mount -> runner.Start
	if len(order) < 3 {
		t.Fatalf("expected at least 3 lifecycle events, got: %v", order)
	}
	if order[0] != "store.Create" || order[1] != "overlay.Mount" || order[2] != "runner.Start" {
		t.Errorf("expected sequence [store.Create, overlay.Mount, runner.Start], got %v", order)
	}

	// Verify MC_ROOTFS passed to child is the merged path, not the base image path
	expectedMerged := "/containers/mockcont1234/merged"
	foundMerged := false
	for _, env := range mockRunner.lastCmd.Env {
		if env == rootfsEnvKey+"="+expectedMerged {
			foundMerged = true
			break
		}
	}
	if !foundMerged {
		t.Errorf("expected %s=%s in cmd.Env, got: %v", rootfsEnvKey, expectedMerged, mockRunner.lastCmd.Env)
	}
}

func TestRunWith_MountsOverlayBeforeStart(t *testing.T) {
	mockRunner := &mockCmdRunner{}
	mockStore := &mockContainerStore{
		createdID:     "c12345678901",
		createdMerged: "/containers/c12345678901/merged",
	}
	mockOverlay := &mockOverlayMounter{}

	cfg := Config{
		Command:  "/bin/sh",
		Hostname: "cow-box",
		Rootfs:   "/images/alpine/rootfs",
	}

	err := runWith(cfg, mockRunner, mockStore, mockOverlay)
	if err != nil {
		t.Fatalf("runWith failed: %v", err)
	}

	if len(mockOverlay.mountCalls) != 1 {
		t.Fatalf("expected 1 mount call, got %d", len(mockOverlay.mountCalls))
	}

	call := mockOverlay.mountCalls[0]
	expectedLower := "/images/alpine/rootfs"
	expectedUpper := "/containers/c12345678901/upper"
	expectedWork := "/containers/c12345678901/work"
	expectedMerged := "/containers/c12345678901/merged"

	if call[0] != expectedLower || call[1] != expectedUpper || call[2] != expectedWork || call[3] != expectedMerged {
		t.Errorf("mount call mismatch:\ngot:  %v\nwant: [%s, %s, %s, %s]", call, expectedLower, expectedUpper, expectedWork, expectedMerged)
	}
}

func TestRunWith_UnmountsAndDestroysOnExit(t *testing.T) {
	var cleanupOrder []string

	mockRunner := &mockCmdRunner{}
	mockStore := &mockContainerStore{
		createdID: "c12345678901",
		onDestroy: func() { cleanupOrder = append(cleanupOrder, "store.Destroy") },
	}
	mockOverlay := &mockOverlayMounter{
		onUnmount: func() { cleanupOrder = append(cleanupOrder, "overlay.Unmount") },
	}

	cfg := Config{
		Command:  "/bin/sh",
		Hostname: "cow-box",
		Rootfs:   "/images/alpine/rootfs",
	}

	err := runWith(cfg, mockRunner, mockStore, mockOverlay)
	if err != nil {
		t.Fatalf("runWith failed: %v", err)
	}

	if len(mockOverlay.unmountCalls) != 1 {
		t.Errorf("expected 1 unmount call, got %d", len(mockOverlay.unmountCalls))
	}
	if len(mockStore.destroyedIDs) != 1 || mockStore.destroyedIDs[0] != "c12345678901" {
		t.Errorf("expected container c12345678901 destroyed, got %v", mockStore.destroyedIDs)
	}

	// Must unmount BEFORE destroying the directory
	if len(cleanupOrder) != 2 || cleanupOrder[0] != "overlay.Unmount" || cleanupOrder[1] != "store.Destroy" {
		t.Errorf("expected cleanup order [overlay.Unmount, store.Destroy], got: %v", cleanupOrder)
	}
}

func TestRunWith_UnmountsOnError(t *testing.T) {
	mockRunner := &mockCmdRunner{
		waitErr: errors.New("process exited with status 1"),
	}
	mockStore := &mockContainerStore{
		createdID: "c-err",
	}
	mockOverlay := &mockOverlayMounter{}

	cfg := Config{
		Command:  "/bin/sh",
		Hostname: "cow-box",
		Rootfs:   "/images/alpine/rootfs",
	}

	err := runWith(cfg, mockRunner, mockStore, mockOverlay)
	if err == nil {
		t.Fatal("expected error from runWith, got nil")
	}

	// Even on error, unmount and destroy must be executed
	if len(mockOverlay.unmountCalls) != 1 {
		t.Errorf("expected 1 unmount call on error, got %d", len(mockOverlay.unmountCalls))
	}
	if len(mockStore.destroyedIDs) != 1 {
		t.Errorf("expected 1 destroy call on error, got %d", len(mockStore.destroyedIDs))
	}
}

func TestRunWith_OverlayCreateError_AbortsBeforeStart(t *testing.T) {
	mockRunner := &mockCmdRunner{}
	mockStore := &mockContainerStore{
		createErr: errors.New("out of space"),
	}
	mockOverlay := &mockOverlayMounter{}

	cfg := Config{
		Command:  "/bin/sh",
		Hostname: "cow-box",
		Rootfs:   "/images/alpine/rootfs",
	}

	err := runWith(cfg, mockRunner, mockStore, mockOverlay)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if mockRunner.lastCmd != nil {
		t.Error("expected runner.Start not to be called when store.Create fails")
	}
	if len(mockOverlay.mountCalls) != 0 {
		t.Error("expected overlay.Mount not to be called when store.Create fails")
	}
}

func TestRunWith_OverlayMountError_CleansUpStoreAndAborts(t *testing.T) {
	mockRunner := &mockCmdRunner{}
	mockStore := &mockContainerStore{
		createdID: "c-mount-fail",
	}
	mockOverlay := &mockOverlayMounter{
		mountErr: errors.New("overlay mount failed"),
	}

	cfg := Config{
		Command:  "/bin/sh",
		Hostname: "cow-box",
		Rootfs:   "/images/alpine/rootfs",
	}

	err := runWith(cfg, mockRunner, mockStore, mockOverlay)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if mockRunner.lastCmd != nil {
		t.Error("expected runner.Start not to be called when overlay.Mount fails")
	}
	if len(mockStore.destroyedIDs) != 1 || mockStore.destroyedIDs[0] != "c-mount-fail" {
		t.Errorf("expected store.Destroy to be called to clean up allocated dir, got %v", mockStore.destroyedIDs)
	}
}


