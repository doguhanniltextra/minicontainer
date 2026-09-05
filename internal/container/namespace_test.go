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

func TestBuildSysProcAttr_HasUserNamespaceFlag(t *testing.T) {
	attr := buildSysProcAttr()

	if attr.Cloneflags&syscall.CLONE_NEWUSER == 0 {
		t.Error("expected CLONE_NEWUSER flag in buildSysProcAttr().Cloneflags")
	}
}

func TestBuildSysProcAttr_HasUIDMapping(t *testing.T) {
	attr := buildSysProcAttr()

	if len(attr.UidMappings) != 1 {
		t.Fatalf("expected 1 UidMapping, got %d", len(attr.UidMappings))
	}

	m := attr.UidMappings[0]
	if m.ContainerID != 0 || m.HostID != 65534 || m.Size != 1 {
		t.Errorf("expected UidMapping {ContainerID: 0, HostID: 65534, Size: 1}, got %+v", m)
	}
}

func TestBuildSysProcAttr_HasGIDMapping(t *testing.T) {
	attr := buildSysProcAttr()

	if len(attr.GidMappings) != 1 {
		t.Fatalf("expected 1 GidMapping, got %d", len(attr.GidMappings))
	}

	m := attr.GidMappings[0]
	if m.ContainerID != 0 || m.HostID != 65534 || m.Size != 1 {
		t.Errorf("expected GidMapping {ContainerID: 0, HostID: 65534, Size: 1}, got %+v", m)
	}
}

func TestBuildSysProcAttrFor_ConfigurableMappings(t *testing.T) {
	cfgWithout := Config{UserNamespace: false}
	attrWithout := buildSysProcAttrFor(cfgWithout)
	if attrWithout.Cloneflags&syscall.CLONE_NEWUSER != 0 {
		t.Error("expected no CLONE_NEWUSER when UserNamespace is false")
	}
	if len(attrWithout.UidMappings) != 0 || len(attrWithout.GidMappings) != 0 {
		t.Error("expected no UID/GID mappings when UserNamespace is false")
	}

	cfgCustom := Config{
		UserNamespace: true,
		HostUID:       1000,
		HostGID:       1000,
	}
	attrCustom := buildSysProcAttrFor(cfgCustom)
	if attrCustom.Cloneflags&syscall.CLONE_NEWUSER == 0 {
		t.Error("expected CLONE_NEWUSER when UserNamespace is true")
	}
	if len(attrCustom.UidMappings) != 1 || attrCustom.UidMappings[0].HostID != 1000 {
		t.Errorf("expected custom HostUID 1000, got %+v", attrCustom.UidMappings)
	}
	if len(attrCustom.GidMappings) != 1 || attrCustom.GidMappings[0].HostID != 1000 {
		t.Errorf("expected custom HostGID 1000, got %+v", attrCustom.GidMappings)
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
