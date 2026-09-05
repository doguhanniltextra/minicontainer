package container

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

type mockFsWriter struct {
	mkdirAllCalls  []string
	writeFileCalls map[string]string
	removeCalls    []string

	mkdirAllErr  error
	writeFileErr error
	removeErr    error
}

func newMockFsWriter() *mockFsWriter {
	return &mockFsWriter{
		writeFileCalls: make(map[string]string),
	}
}

func (m *mockFsWriter) MkdirAll(path string, perm fs.FileMode) error {
	m.mkdirAllCalls = append(m.mkdirAllCalls, path)
	return m.mkdirAllErr
}

func (m *mockFsWriter) WriteFile(name string, data []byte, perm fs.FileMode) error {
	if m.writeFileErr != nil {
		return m.writeFileErr
	}
	m.writeFileCalls[name] = string(data)
	return nil
}

func (m *mockFsWriter) Remove(name string) error {
	m.removeCalls = append(m.removeCalls, name)
	return m.removeErr
}

func (m *mockFsWriter) RemoveAll(name string) error {
	m.removeCalls = append(m.removeCalls, name)
	return m.removeErr
}

func TestNewCgroupManager_CreatesUniqueDirectory(t *testing.T) {
	mock := newMockFsWriter()
	cm, err := newCgroupManagerWith(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.mkdirAllCalls) != 1 {
		t.Fatalf("expected 1 MkdirAll call, got %d", len(mock.mkdirAllCalls))
	}

	createdPath := mock.mkdirAllCalls[0]
	if !strings.HasPrefix(createdPath, cgroupRoot+"/minicontainer-") {
		t.Errorf("expected path prefix %q, got %q", cgroupRoot+"/minicontainer-", createdPath)
	}
	if cm.path != createdPath {
		t.Errorf("expected manager path %q, got %q", createdPath, cm.path)
	}
}

func TestNewCgroupManager_MkdirError_Aborts(t *testing.T) {
	mock := newMockFsWriter()
	mock.mkdirAllErr = errors.New("permission denied")

	_, err := newCgroupManagerWith(mock)
	if err == nil {
		t.Fatal("expected error on MkdirAll failure, got nil")
	}
}

func TestApply_WritesMemoryLimit(t *testing.T) {
	mock := newMockFsWriter()
	cm, err := newCgroupManagerWithPath(mock, "/sys/fs/cgroup/minicontainer-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := Config{
		MemoryLimit: 104857600,
	}

	if err := cm.apply(cfg); err != nil {
		t.Fatalf("unexpected apply error: %v", err)
	}

	expectedFile := filepath.Join(cm.path, "memory.max")
	val, ok := mock.writeFileCalls[expectedFile]
	if !ok {
		t.Fatalf("expected %s to be written", expectedFile)
	}
	if val != "104857600" {
		t.Errorf("expected memory.max %q, got %q", "104857600", val)
	}
}

func TestApply_WritesPidsLimit(t *testing.T) {
	mock := newMockFsWriter()
	cm, err := newCgroupManagerWithPath(mock, "/sys/fs/cgroup/minicontainer-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := Config{
		PidsLimit: 20,
	}

	if err := cm.apply(cfg); err != nil {
		t.Fatalf("unexpected apply error: %v", err)
	}

	expectedFile := filepath.Join(cm.path, "pids.max")
	val, ok := mock.writeFileCalls[expectedFile]
	if !ok {
		t.Fatalf("expected %s to be written", expectedFile)
	}
	if val != "20" {
		t.Errorf("expected pids.max %q, got %q", "20", val)
	}
}

func TestApply_WritesCpuMax(t *testing.T) {
	mock := newMockFsWriter()
	cm, err := newCgroupManagerWithPath(mock, "/sys/fs/cgroup/minicontainer-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := Config{
		CpuQuota:  50000,
		CpuPeriod: 100000,
	}

	if err := cm.apply(cfg); err != nil {
		t.Fatalf("unexpected apply error: %v", err)
	}

	expectedFile := filepath.Join(cm.path, "cpu.max")
	val, ok := mock.writeFileCalls[expectedFile]
	if !ok {
		t.Fatalf("expected %s to be written", expectedFile)
	}
	if val != "50000 100000" {
		t.Errorf("expected cpu.max %q, got %q", "50000 100000", val)
	}
}

func TestApply_WritesCpuMax_DefaultPeriodWhenZero(t *testing.T) {
	mock := newMockFsWriter()
	cm, err := newCgroupManagerWithPath(mock, "/sys/fs/cgroup/minicontainer-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := Config{
		CpuQuota:  50000,
		CpuPeriod: 0, // Should default to 100000
	}

	if err := cm.apply(cfg); err != nil {
		t.Fatalf("unexpected apply error: %v", err)
	}

	expectedFile := filepath.Join(cm.path, "cpu.max")
	val, ok := mock.writeFileCalls[expectedFile]
	if !ok {
		t.Fatalf("expected %s to be written", expectedFile)
	}
	if val != "50000 100000" {
		t.Errorf("expected cpu.max %q, got %q", "50000 100000", val)
	}
}

func TestApply_SkipsZeroValues(t *testing.T) {
	mock := newMockFsWriter()
	cm, err := newCgroupManagerWithPath(mock, "/sys/fs/cgroup/minicontainer-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := Config{
		MemoryLimit: 0,
		PidsLimit:   0,
		CpuQuota:    0,
	}

	if err := cm.apply(cfg); err != nil {
		t.Fatalf("unexpected apply error: %v", err)
	}

	if len(mock.writeFileCalls) != 0 {
		t.Errorf("expected no files written for zero limits, wrote %v", mock.writeFileCalls)
	}
}

func TestAddProcess_WritesPidToCgroupProcs(t *testing.T) {
	mock := newMockFsWriter()
	cm, err := newCgroupManagerWithPath(mock, "/sys/fs/cgroup/minicontainer-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := cm.addProcess(12345); err != nil {
		t.Fatalf("unexpected addProcess error: %v", err)
	}

	expectedFile := filepath.Join(cm.path, "cgroup.procs")
	val, ok := mock.writeFileCalls[expectedFile]
	if !ok {
		t.Fatalf("expected %s to be written", expectedFile)
	}
	if val != "12345" {
		t.Errorf("expected cgroup.procs %q, got %q", "12345", val)
	}
}

func TestCleanup_RemovesDirectory(t *testing.T) {
	mock := newMockFsWriter()
	cm, err := newCgroupManagerWithPath(mock, "/sys/fs/cgroup/minicontainer-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := cm.cleanup(); err != nil {
		t.Fatalf("unexpected cleanup error: %v", err)
	}

	if len(mock.removeCalls) != 1 {
		t.Fatalf("expected 1 Remove call, got %d", len(mock.removeCalls))
	}
	if mock.removeCalls[0] != cm.path {
		t.Errorf("expected Remove %q, got %q", cm.path, mock.removeCalls[0])
	}
}

func TestApply_ErrorPropagation(t *testing.T) {
	testCases := []struct {
		name string
		cfg  Config
	}{
		{
			name: "memory.max error",
			cfg:  Config{MemoryLimit: 104857600},
		},
		{
			name: "pids.max error",
			cfg:  Config{PidsLimit: 20},
		},
		{
			name: "cpu.max error",
			cfg:  Config{CpuQuota: 50000},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mock := newMockFsWriter()
			cm, err := newCgroupManagerWithPath(mock, "/sys/fs/cgroup/minicontainer-test")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			mock.writeFileErr = errors.New("write failed")
			err = cm.apply(tc.cfg)
			if err == nil {
				t.Fatal("expected error on write failure, got nil")
			}
		})
	}
}

func TestAddProcess_ErrorPropagation(t *testing.T) {
	mock := newMockFsWriter()
	cm, err := newCgroupManagerWithPath(mock, "/sys/fs/cgroup/minicontainer-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mock.writeFileErr = errors.New("write failed")
	if err := cm.addProcess(12345); err == nil {
		t.Fatal("expected error on write failure, got nil")
	}
}

func TestCleanup_ErrorPropagation(t *testing.T) {
	mock := newMockFsWriter()
	cm, err := newCgroupManagerWithPath(mock, "/sys/fs/cgroup/minicontainer-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mock.removeErr = errors.New("remove failed")
	if err := cm.cleanup(); err == nil {
		t.Fatal("expected error on remove failure, got nil")
	}
}
