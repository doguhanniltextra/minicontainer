package container

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
)

// mockPivotRooter tracks calls to PivotRoot and Chdir for unit testing.
type mockPivotRooter struct {
	lastNewRoot string
	lastPutOld  string
	lastChdir   string
	pivotErr    error
	chdirErr    error
}

func (m *mockPivotRooter) PivotRoot(newRoot, putOld string) error {
	m.lastNewRoot = newRoot
	m.lastPutOld = putOld
	return m.pivotErr
}

func (m *mockPivotRooter) Chdir(dir string) error {
	m.lastChdir = dir
	return m.chdirErr
}

func TestPivotRoot_Success(t *testing.T) {
	tempRootfs := t.TempDir()
	mockM := &mockMounter{}
	mockP := &mockPivotRooter{}

	err := pivotRoot(tempRootfs, mockM, mockP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1. Must make mount namespace private
	if len(mockM.mountCalls) < 2 {
		t.Fatalf("expected at least 2 mount calls, got %d", len(mockM.mountCalls))
	}
	privateCall := mockM.mountCalls[0]
	if privateCall.target != "/" || privateCall.flags != (syscall.MS_REC|syscall.MS_PRIVATE) {
		t.Errorf("expected private mount on /, got %+v", privateCall)
	}

	// 2. Must bind-mount newRoot onto itself
	bindCall := mockM.mountCalls[1]
	if bindCall.source != tempRootfs || bindCall.target != tempRootfs || bindCall.flags != (syscall.MS_BIND|syscall.MS_REC) {
		t.Errorf("expected self bind-mount on %s, got %+v", tempRootfs, bindCall)
	}

	// 3. Must execute PivotRoot
	expectedOldRoot := filepath.Join(tempRootfs, oldRootName)
	if mockP.lastNewRoot != tempRootfs || mockP.lastPutOld != expectedOldRoot {
		t.Errorf("expected PivotRoot(%q, %q), got (%q, %q)", tempRootfs, expectedOldRoot, mockP.lastNewRoot, mockP.lastPutOld)
	}

	// 4. Must chdir to /
	if mockP.lastChdir != "/" {
		t.Errorf("expected chdir to /, got %q", mockP.lastChdir)
	}

	// 5. Must unmount old root with MNT_DETACH
	if len(mockM.unmountCalls) != 1 {
		t.Fatalf("expected 1 unmount call, got %d", len(mockM.unmountCalls))
	}
	unmountCall := mockM.unmountCalls[0]
	if unmountCall.target != "/"+oldRootName || unmountCall.flags != syscall.MNT_DETACH {
		t.Errorf("expected unmount of /%s with MNT_DETACH, got %+v", oldRootName, unmountCall)
	}
}

func TestPivotRoot_PropagationError_Aborts(t *testing.T) {
	tempRootfs := t.TempDir()
	expectedErr := errors.New("cannot make mounts private")
	mockM := &mockMounter{mountErr: expectedErr}
	mockP := &mockPivotRooter{}

	err := pivotRoot(tempRootfs, mockM, mockP)
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	if mockP.lastNewRoot != "" {
		t.Errorf("expected no PivotRoot call on mount propagation failure")
	}
}

func TestPivotRoot_PivotSyscallError_Aborts(t *testing.T) {
	tempRootfs := t.TempDir()
	expectedErr := errors.New("invalid argument")
	mockM := &mockMounter{}
	mockP := &mockPivotRooter{pivotErr: expectedErr}

	err := pivotRoot(tempRootfs, mockM, mockP)
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	if mockP.lastChdir != "" {
		t.Errorf("expected no chdir call on pivot_root failure")
	}
}

func TestPivotRoot_ChdirError_Aborts(t *testing.T) {
	tempRootfs := t.TempDir()
	expectedErr := errors.New("cannot change directory")
	mockM := &mockMounter{}
	mockP := &mockPivotRooter{chdirErr: expectedErr}

	err := pivotRoot(tempRootfs, mockM, mockP)
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	if len(mockM.unmountCalls) != 0 {
		t.Errorf("expected no unmount call on chdir failure")
	}
}
