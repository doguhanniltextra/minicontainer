package container

import (
	"errors"
	"syscall"
	"testing"
)

// mockHostnamer is a test double for the hostnamer interface.
// It records calls and can be configured to return an error.
type mockHostnamer struct {
	lastHostname string
	err          error
}

func (m *mockHostnamer) Sethostname(name []byte) error {
	m.lastHostname = string(name)
	return m.err
}

// --- buildSysProcAttr tests ---

func TestBuildSysProcAttr_HasAllRequiredFlags(t *testing.T) {
	attr := buildSysProcAttr()

	requiredFlags := map[string]uintptr{
		"CLONE_NEWPID": syscall.CLONE_NEWPID,
		"CLONE_NEWUTS": syscall.CLONE_NEWUTS,
		"CLONE_NEWNET": syscall.CLONE_NEWNET,
		"CLONE_NEWNS":  syscall.CLONE_NEWNS,
	}

	for name, flag := range requiredFlags {
		if attr.Cloneflags&flag == 0 {
			t.Errorf("missing required namespace flag: %s", name)
		}
	}
}

func TestBuildSysProcAttr_ReturnsNonNil(t *testing.T) {
	attr := buildSysProcAttr()

	if attr == nil {
		t.Fatal("buildSysProcAttr() returned nil")
	}
}

// --- setHostname tests ---

func TestSetHostname_CallsSethostname(t *testing.T) {
	mock := &mockHostnamer{}

	err := setHostname(mock, "minicontainer")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.lastHostname != "minicontainer" {
		t.Errorf("expected hostname %q, got %q", "minicontainer", mock.lastHostname)
	}
}

func TestSetHostname_PropagatesError(t *testing.T) {
	expected := errors.New("operation not permitted")
	mock := &mockHostnamer{err: expected}

	err := setHostname(mock, "minicontainer")

	if err != expected {
		t.Errorf("expected error %v, got %v", expected, err)
	}
}

func TestSetHostname_PassesCorrectBytes(t *testing.T) {
	mock := &mockHostnamer{}

	_ = setHostname(mock, "test-host")

	if mock.lastHostname != "test-host" {
		t.Errorf("expected %q to be passed as bytes, got %q", "test-host", mock.lastHostname)
	}
}
