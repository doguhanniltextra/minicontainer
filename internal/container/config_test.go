package container

import "testing"

func TestConfig_DefaultZeroValues(t *testing.T) {
	cfg := Config{}

	if cfg.Command != "" {
		t.Errorf("zero-value Command should be empty string, got %q", cfg.Command)
	}
	if cfg.Args != nil {
		t.Errorf("zero-value Args should be nil, got %v", cfg.Args)
	}
	if cfg.Hostname != "" {
		t.Errorf("zero-value Hostname should be empty string, got %q", cfg.Hostname)
	}
	if cfg.Rootfs != "" {
		t.Errorf("zero-value Rootfs should be empty string, got %q", cfg.Rootfs)
	}
	if cfg.MemoryLimit != 0 {
		t.Errorf("zero-value MemoryLimit should be 0, got %d", cfg.MemoryLimit)
	}
	if cfg.PidsLimit != 0 {
		t.Errorf("zero-value PidsLimit should be 0, got %d", cfg.PidsLimit)
	}
	if cfg.CpuQuota != 0 {
		t.Errorf("zero-value CpuQuota should be 0, got %d", cfg.CpuQuota)
	}
	if cfg.CpuPeriod != 0 {
		t.Errorf("zero-value CpuPeriod should be 0, got %d", cfg.CpuPeriod)
	}
}

func TestConfig_FieldAssignment(t *testing.T) {
	cfg := Config{
		Command:     "/bin/bash",
		Args:        []string{"-i"},
		Hostname:    "minicontainer",
		Rootfs:      "assets/rootfs",
		MemoryLimit: 104857600,
		PidsLimit:   20,
		CpuQuota:    50000,
		CpuPeriod:   100000,
	}

	if cfg.Command != "/bin/bash" {
		t.Errorf("expected Command %q, got %q", "/bin/bash", cfg.Command)
	}
	if len(cfg.Args) != 1 || cfg.Args[0] != "-i" {
		t.Errorf("expected Args ['-i'], got %v", cfg.Args)
	}
	if cfg.Hostname != "minicontainer" {
		t.Errorf("expected Hostname %q, got %q", "minicontainer", cfg.Hostname)
	}
	if cfg.Rootfs != "assets/rootfs" {
		t.Errorf("expected Rootfs %q, got %q", "assets/rootfs", cfg.Rootfs)
	}
	if cfg.MemoryLimit != 104857600 {
		t.Errorf("expected MemoryLimit 104857600, got %d", cfg.MemoryLimit)
	}
	if cfg.PidsLimit != 20 {
		t.Errorf("expected PidsLimit 20, got %d", cfg.PidsLimit)
	}
	if cfg.CpuQuota != 50000 {
		t.Errorf("expected CpuQuota 50000, got %d", cfg.CpuQuota)
	}
	if cfg.CpuPeriod != 100000 {
		t.Errorf("expected CpuPeriod 100000, got %d", cfg.CpuPeriod)
	}
}

func TestConfig_EmptyArgs(t *testing.T) {
	cfg := Config{
		Command:  "/bin/sh",
		Args:     []string{},
		Hostname: "minicontainer",
		Rootfs:   "assets/rootfs",
	}

	// An explicitly set empty slice should have length 0
	if len(cfg.Args) != 0 {
		t.Errorf("expected empty Args slice, got length %d", len(cfg.Args))
	}
}

func TestConfig_HasLimits(t *testing.T) {
	if (Config{}).hasLimits() {
		t.Error("expected hasLimits to be false for zero Config")
	}

	if !(Config{MemoryLimit: 104857600}).hasLimits() {
		t.Error("expected hasLimits to be true when MemoryLimit > 0")
	}

	if !(Config{PidsLimit: 20}).hasLimits() {
		t.Error("expected hasLimits to be true when PidsLimit > 0")
	}

	if !(Config{CpuQuota: 50000}).hasLimits() {
		t.Error("expected hasLimits to be true when CpuQuota > 0")
	}

	// CpuPeriod alone without CpuQuota does not constitute active limits
	if (Config{CpuPeriod: 100000}).hasLimits() {
		t.Error("expected hasLimits to be false when only CpuPeriod is set")
	}
}
