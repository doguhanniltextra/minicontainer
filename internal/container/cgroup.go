package container

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
	"strconv"
)

const (
	cgroupRoot         = "/sys/fs/cgroup"
	memoryLimitFile    = "memory.max"
	pidsLimitFile      = "pids.max"
	cpuLimitFile       = "cpu.max"
	cgroupProcsFile    = "cgroup.procs"
	defaultCpuPeriodUs = 100000 // 100ms in microseconds
)

// realCgroupManager manages the lifecycle and resource limits of a Linux cgroup v2 directory.
type realCgroupManager struct {
	path string
	fs   fsWriter
}

// newCgroupManager creates a unique cgroup directory under /sys/fs/cgroup and returns a manager.
func newCgroupManager() (cgroupManager, error) {
	return newCgroupManagerWith(realFsWriter{})
}

// newCgroupManagerWith creates a realCgroupManager with an injected fsWriter implementation.
func newCgroupManagerWith(fs fsWriter) (*realCgroupManager, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generating cgroup id: %w", err)
	}

	cgroupName := fmt.Sprintf("minicontainer-%x", b)
	path := filepath.Join(cgroupRoot, cgroupName)
	return newCgroupManagerWithPath(fs, path)
}

// newCgroupManagerWithPath creates a cgroup directory at the exact path specified using the injected fsWriter.
func newCgroupManagerWithPath(fs fsWriter, path string) (*realCgroupManager, error) {
	if err := fs.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("creating cgroup directory %s: %w", path, err)
	}

	return &realCgroupManager{
		path: path,
		fs:   fs,
	}, nil
}

// apply writes configured resource limits from Config to the appropriate cgroup v2 limit files.
// Fields with value 0 are skipped (unlimited).
func (c *realCgroupManager) apply(cfg Config) error {
	if cfg.MemoryLimit > 0 {
		memFile := filepath.Join(c.path, memoryLimitFile)
		val := strconv.FormatInt(cfg.MemoryLimit, 10)
		if err := c.fs.WriteFile(memFile, []byte(val), 0644); err != nil {
			return fmt.Errorf("setting memory limit: %w", err)
		}
	}

	if cfg.PidsLimit > 0 {
		pidsFile := filepath.Join(c.path, pidsLimitFile)
		val := strconv.FormatInt(cfg.PidsLimit, 10)
		if err := c.fs.WriteFile(pidsFile, []byte(val), 0644); err != nil {
			return fmt.Errorf("setting pids limit: %w", err)
		}
	}

	if cfg.CpuQuota > 0 {
		period := cfg.CpuPeriod
		if period <= 0 {
			period = defaultCpuPeriodUs
		}
		cpuFile := filepath.Join(c.path, cpuLimitFile)
		val := fmt.Sprintf("%d %d", cfg.CpuQuota, period)
		if err := c.fs.WriteFile(cpuFile, []byte(val), 0644); err != nil {
			return fmt.Errorf("setting cpu limit: %w", err)
		}
	}

	return nil
}

// addProcess writes the specified PID to cgroup.procs, attaching the process
// and all its future child processes to this cgroup.
func (c *realCgroupManager) addProcess(pid int) error {
	procsFile := filepath.Join(c.path, cgroupProcsFile)
	val := strconv.Itoa(pid)
	if err := c.fs.WriteFile(procsFile, []byte(val), 0644); err != nil {
		return fmt.Errorf("adding process %d to cgroup: %w", pid, err)
	}
	return nil
}

// cleanup removes the cgroup directory. It should be called in a defer
// statement once the container process exits.
func (c *realCgroupManager) cleanup() error {
	if err := c.fs.Remove(c.path); err != nil {
		return fmt.Errorf("removing cgroup %s: %w", c.path, err)
	}
	return nil
}

