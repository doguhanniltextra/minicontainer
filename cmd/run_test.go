package cmd

import (
	"errors"
	"testing"

	"minicontainer/internal/container"
)

func TestNewRunCmd_ParsesCommandAndArgs_DefaultRootfs(t *testing.T) {
	var capturedCfg container.Config
	mockRunner := func(cfg container.Config) error {
		capturedCfg = cfg
		return nil
	}

	cmd := newRunCmd(mockRunner)
	cmd.SetArgs([]string{"/bin/bash", "-c", "echo test"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedCfg.Command != "/bin/bash" {
		t.Errorf("expected Command %q, got %q", "/bin/bash", capturedCfg.Command)
	}
	if len(capturedCfg.Args) != 2 || capturedCfg.Args[0] != "-c" || capturedCfg.Args[1] != "echo test" {
		t.Errorf("expected Args ['-c', 'echo test'], got %v", capturedCfg.Args)
	}
	if capturedCfg.Hostname != "minicontainer" {
		t.Errorf("expected Hostname %q, got %q", "minicontainer", capturedCfg.Hostname)
	}
	if capturedCfg.Rootfs != defaultRootfs {
		t.Errorf("expected Rootfs default %q, got %q", defaultRootfs, capturedCfg.Rootfs)
	}
}

func TestNewRunCmd_ParsesCustomRootfsLongFlag(t *testing.T) {
	var capturedCfg container.Config
	mockRunner := func(cfg container.Config) error {
		capturedCfg = cfg
		return nil
	}

	cmd := newRunCmd(mockRunner)
	cmd.SetArgs([]string{"--rootfs", "/tmp/alpine-rootfs", "/bin/sh", "-c", "echo hello"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedCfg.Rootfs != "/tmp/alpine-rootfs" {
		t.Errorf("expected Rootfs %q, got %q", "/tmp/alpine-rootfs", capturedCfg.Rootfs)
	}
	if capturedCfg.Command != "/bin/sh" {
		t.Errorf("expected Command /bin/sh, got %q", capturedCfg.Command)
	}
	if len(capturedCfg.Args) != 2 || capturedCfg.Args[0] != "-c" || capturedCfg.Args[1] != "echo hello" {
		t.Errorf("expected Args ['-c', 'echo hello'], got %v", capturedCfg.Args)
	}
}

func TestNewRunCmd_ParsesCustomRootfsShortFlag(t *testing.T) {
	var capturedCfg container.Config
	mockRunner := func(cfg container.Config) error {
		capturedCfg = cfg
		return nil
	}

	cmd := newRunCmd(mockRunner)
	cmd.SetArgs([]string{"-r", "/tmp/custom-rootfs", "/bin/sh"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedCfg.Rootfs != "/tmp/custom-rootfs" {
		t.Errorf("expected Rootfs %q, got %q", "/tmp/custom-rootfs", capturedCfg.Rootfs)
	}
	if capturedCfg.Command != "/bin/sh" {
		t.Errorf("expected Command /bin/sh, got %q", capturedCfg.Command)
	}
}

func TestNewRunCmd_HelpFlagDoesNotRunContainer(t *testing.T) {
	runnerCalled := false
	mockRunner := func(cfg container.Config) error {
		runnerCalled = true
		return nil
	}

	cmd := newRunCmd(mockRunner)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runnerCalled {
		t.Error("expected container runner NOT to be called on --help")
	}
}

func TestNewRunCmd_RequiresAtLeastOneArg(t *testing.T) {
	mockRunner := func(cfg container.Config) error {
		return nil
	}

	cmd := newRunCmd(mockRunner)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no arguments provided, got nil")
	}
}

func TestNewRunCmd_PropagatesRunnerError(t *testing.T) {
	expectedErr := errors.New("container execution failed")
	mockRunner := func(cfg container.Config) error {
		return expectedErr
	}

	cmd := newRunCmd(mockRunner)
	cmd.SetArgs([]string{"/bin/bash"})

	err := cmd.Execute()
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}
