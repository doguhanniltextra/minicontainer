package container

import (
	"errors"
	"testing"
)

// mockMounter is a test double for the mounter interface.
// It tracks calls to Mount and Unmount and can return configured errors.
type mockMounter struct {
	mountCalls   []mountCall
	unmountCalls []unmountCall
	mountErr     error
	unmountErr   error
}

type mountCall struct {
	source string
	target string
	fstype string
	flags  uintptr
	data   string
}

type unmountCall struct {
	target string
	flags  int
}

func (m *mockMounter) Mount(source, target, fstype string, flags uintptr, data string) error {
	m.mountCalls = append(m.mountCalls, mountCall{
		source: source,
		target: target,
		fstype: fstype,
		flags:  flags,
		data:   data,
	})
	return m.mountErr
}

func (m *mockMounter) Unmount(target string, flags int) error {
	m.unmountCalls = append(m.unmountCalls, unmountCall{
		target: target,
		flags:  flags,
	})
	return m.unmountErr
}

// --- mountProc tests ---

func TestMountProc_CallsMountWithCorrectArguments(t *testing.T) {
	mock := &mockMounter{}

	err := mountProc(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.mountCalls) != 1 {
		t.Fatalf("expected 1 call to Mount, got %d", len(mock.mountCalls))
	}

	call := mock.mountCalls[0]
	if call.source != "proc" {
		t.Errorf("expected source %q, got %q", "proc", call.source)
	}
	if call.target != "/proc" {
		t.Errorf("expected target %q, got %q", "/proc", call.target)
	}
	if call.fstype != "proc" {
		t.Errorf("expected fstype %q, got %q", "proc", call.fstype)
	}
}

func TestMountProc_PropagatesError(t *testing.T) {
	expectedErr := errors.New("permission denied")
	mock := &mockMounter{mountErr: expectedErr}

	err := mountProc(mock)
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

// --- mountDev tests ---

func TestMountDev_CallsMountWithCorrectArguments(t *testing.T) {
	mock := &mockMounter{}

	err := mountDev(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.mountCalls) != 1 {
		t.Fatalf("expected 1 call to Mount, got %d", len(mock.mountCalls))
	}

	call := mock.mountCalls[0]
	if call.target != "/dev" {
		t.Errorf("expected target /dev, got %q", call.target)
	}
	if call.fstype != "devtmpfs" && call.fstype != "tmpfs" {
		t.Errorf("expected fstype devtmpfs or tmpfs, got %q", call.fstype)
	}
}

func TestMountDev_PropagatesError(t *testing.T) {
	expectedErr := errors.New("dev mount failed")
	mock := &mockMounter{mountErr: expectedErr}

	err := mountDev(mock)
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

// --- mountTmp tests ---

func TestMountTmp_CallsMountWithCorrectArguments(t *testing.T) {
	mock := &mockMounter{}

	err := mountTmp(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.mountCalls) != 1 {
		t.Fatalf("expected 1 call to Mount, got %d", len(mock.mountCalls))
	}

	call := mock.mountCalls[0]
	if call.target != "/tmp" {
		t.Errorf("expected target /tmp, got %q", call.target)
	}
	if call.fstype != "tmpfs" {
		t.Errorf("expected fstype tmpfs, got %q", call.fstype)
	}
}

// --- mountVirtualFilesystems tests ---

func TestMountVirtualFilesystems_MountsAll(t *testing.T) {
	mock := &mockMounter{}

	err := mountVirtualFilesystems(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.mountCalls) != 3 {
		t.Fatalf("expected 3 mount calls (/proc, /dev, /tmp), got %d", len(mock.mountCalls))
	}
	targets := []string{mock.mountCalls[0].target, mock.mountCalls[1].target, mock.mountCalls[2].target}
	expected := []string{"/proc", "/dev", "/tmp"}
	for i, exp := range expected {
		if targets[i] != exp {
			t.Errorf("expected mount target %d to be %q, got %q", i, exp, targets[i])
		}
	}
}

// --- unmountVirtualFilesystems tests ---

func TestUnmountVirtualFilesystems_UnmountsAll(t *testing.T) {
	mock := &mockMounter{}

	err := unmountVirtualFilesystems(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.unmountCalls) != 3 {
		t.Fatalf("expected 3 unmount calls, got %d", len(mock.unmountCalls))
	}
}
