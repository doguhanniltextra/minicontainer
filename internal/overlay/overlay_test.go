package overlay

import (
	"errors"
	"strings"
	"syscall"
	"testing"
)

type mockMountSyscaller struct {
	lastSource string
	lastTarget string
	lastFstype string
	lastFlags  uintptr
	lastData   string
	mountErr   error

	unmountTarget string
	unmountFlags  int
	unmountErr    error
}

func (m *mockMountSyscaller) Mount(source, target, fstype string, flags uintptr, data string) error {
	m.lastSource = source
	m.lastTarget = target
	m.lastFstype = fstype
	m.lastFlags = flags
	m.lastData = data
	return m.mountErr
}

func (m *mockMountSyscaller) Unmount(target string, flags int) error {
	m.unmountTarget = target
	m.unmountFlags = flags
	return m.unmountErr
}

func TestMount_CallsSyscallWithCorrectOptions(t *testing.T) {
	mock := &mockMountSyscaller{}
	mounter := newWithSyscaller(mock)

	lower := "/images/alpine/rootfs"
	upper := "/containers/c1/upper"
	work := "/containers/c1/work"
	merged := "/containers/c1/merged"

	err := mounter.Mount(lower, upper, work, merged)
	if err != nil {
		t.Fatalf("expected Mount to succeed, got: %v", err)
	}

	if mock.lastSource != "overlay" {
		t.Errorf("expected source 'overlay', got %q", mock.lastSource)
	}
	if mock.lastTarget != merged {
		t.Errorf("expected target %q, got %q", merged, mock.lastTarget)
	}
	if mock.lastFstype != "overlay" {
		t.Errorf("expected fstype 'overlay', got %q", mock.lastFstype)
	}
	if mock.lastFlags != 0 {
		t.Errorf("expected flags 0, got %d", mock.lastFlags)
	}

	expectedOpts := "lowerdir=" + lower + ",upperdir=" + upper + ",workdir=" + work
	if mock.lastData != expectedOpts {
		t.Errorf("expected opts %q, got %q", expectedOpts, mock.lastData)
	}
}

func TestMount_ErrorPropagation(t *testing.T) {
	mock := &mockMountSyscaller{
		mountErr: syscall.EPERM,
	}
	mounter := newWithSyscaller(mock)

	err := mounter.Mount("/l", "/u", "/w", "/m")
	if err == nil {
		t.Fatal("expected error on mount failure, got nil")
	}
	if !errors.Is(err, syscall.EPERM) {
		t.Errorf("expected syscall.EPERM in error chain, got: %v", err)
	}
}

func TestMount_ValidatesEmptyParameters(t *testing.T) {
	mock := &mockMountSyscaller{}
	mounter := newWithSyscaller(mock)

	cases := []struct {
		lower, upper, work, merged string
	}{
		{"", "/u", "/w", "/m"},
		{"/l", "", "/w", "/m"},
		{"/l", "/u", "", "/m"},
		{"/l", "/u", "/w", ""},
	}

	for _, tc := range cases {
		err := mounter.Mount(tc.lower, tc.upper, tc.work, tc.merged)
		if err == nil {
			t.Errorf("expected error for empty arg in %+v, got nil", tc)
		}
	}
}

func TestUnmount_CallsSyscall(t *testing.T) {
	mock := &mockMountSyscaller{}
	mounter := newWithSyscaller(mock)

	merged := "/containers/c1/merged"
	err := mounter.Unmount(merged)
	if err != nil {
		t.Fatalf("expected Unmount to succeed, got: %v", err)
	}

	if mock.unmountTarget != merged {
		t.Errorf("expected unmount target %q, got %q", merged, mock.unmountTarget)
	}
	if mock.unmountFlags != 0 {
		t.Errorf("expected unmount flags 0, got %d", mock.unmountFlags)
	}
}

func TestUnmount_ErrorPropagation(t *testing.T) {
	mock := &mockMountSyscaller{
		unmountErr: syscall.EINVAL,
	}
	mounter := newWithSyscaller(mock)

	err := mounter.Unmount("/containers/c1/merged")
	if err == nil {
		t.Fatal("expected error on unmount failure, got nil")
	}
	if !errors.Is(err, syscall.EINVAL) {
		t.Errorf("expected syscall.EINVAL in error chain, got: %v", err)
	}
}

func TestUnmount_EmptyMergedDir(t *testing.T) {
	mock := &mockMountSyscaller{}
	mounter := newWithSyscaller(mock)

	err := mounter.Unmount("")
	if err == nil {
		t.Fatal("expected error for empty mergeddir, got nil")
	}
	if !strings.Contains(err.Error(), "mergeddir") {
		t.Errorf("expected error to mention mergeddir, got: %v", err)
	}
}

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("expected non-nil Mounter from New()")
	}
}
