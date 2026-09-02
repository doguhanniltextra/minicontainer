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
}

func TestConfig_FieldAssignment(t *testing.T) {
	cfg := Config{
		Command:  "/bin/bash",
		Args:     []string{"-i"},
		Hostname: "minicontainer",
		Rootfs:   "assets/rootfs",
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
