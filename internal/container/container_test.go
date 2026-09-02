package container

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"
)

// mockCmdRunner records the last command passed to Run.
type mockCmdRunner struct {
	lastCmd *exec.Cmd
	runErr  error
}

func (m *mockCmdRunner) Run(cmd *exec.Cmd) error {
	m.lastCmd = cmd
	return m.runErr
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
