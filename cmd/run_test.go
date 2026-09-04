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

func TestParseMemory(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{name: "empty string (unlimited)", input: "", want: 0, wantErr: false},
		{name: "zero value", input: "0", want: 0, wantErr: false},
		{name: "lowercase megabytes 100m", input: "100m", want: 104857600, wantErr: false},
		{name: "uppercase megabytes 100M", input: "100M", want: 104857600, wantErr: false},
		{name: "512m", input: "512m", want: 536870912, wantErr: false},
		{name: "1g", input: "1g", want: 1073741824, wantErr: false},
		{name: "1G", input: "1G", want: 1073741824, wantErr: false},
		{name: "kilobytes 64k", input: "64k", want: 65536, wantErr: false},
		{name: "kilobytes 64K", input: "64K", want: 65536, wantErr: false},
		{name: "bytes 1024b", input: "1024b", want: 1024, wantErr: false},
		{name: "bytes 1024B", input: "1024B", want: 1024, wantErr: false},
		{name: "plain number bytes", input: "1024", want: 1024, wantErr: false},
		{name: "invalid suffix", input: "100x", want: 0, wantErr: true},
		{name: "non-numeric string", input: "invalid", want: 0, wantErr: true},
		{name: "negative value", input: "-100m", want: 0, wantErr: true},
		{name: "negative plain number", input: "-1", want: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMemory(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseMemory(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseMemory(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewRunCmd_ParsesMemoryFlags(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want int64
	}{
		{
			name: "long flag space-separated",
			args: []string{"--memory", "100m", "/bin/sh"},
			want: 104857600,
		},
		{
			name: "long flag equal-separated",
			args: []string{"--memory=256m", "/bin/sh"},
			want: 268435456,
		},
		{
			name: "short flag space-separated",
			args: []string{"-m", "512m", "/bin/sh"},
			want: 536870912,
		},
		{
			name: "short flag equal-separated",
			args: []string{"-m=1g", "/bin/sh"},
			want: 1073741824,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedCfg container.Config
			cmd := newRunCmd(func(cfg container.Config) error {
				capturedCfg = cfg
				return nil
			})
			cmd.SetArgs(tc.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if capturedCfg.MemoryLimit != tc.want {
				t.Errorf("expected MemoryLimit %d, got %d", tc.want, capturedCfg.MemoryLimit)
			}
		})
	}
}

func TestNewRunCmd_ParsesPidsFlags(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want int64
	}{
		{
			name: "long flag space-separated",
			args: []string{"--pids", "30", "/bin/sh"},
			want: 30,
		},
		{
			name: "long flag equal-separated",
			args: []string{"--pids=50", "/bin/sh"},
			want: 50,
		},
		{
			name: "short flag space-separated",
			args: []string{"-p", "20", "/bin/sh"},
			want: 20,
		},
		{
			name: "short flag equal-separated",
			args: []string{"-p=100", "/bin/sh"},
			want: 100,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedCfg container.Config
			cmd := newRunCmd(func(cfg container.Config) error {
				capturedCfg = cfg
				return nil
			})
			cmd.SetArgs(tc.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if capturedCfg.PidsLimit != tc.want {
				t.Errorf("expected PidsLimit %d, got %d", tc.want, capturedCfg.PidsLimit)
			}
		})
	}
}

func TestNewRunCmd_ParsesCpusFlags(t *testing.T) {
	testCases := []struct {
		name       string
		args       []string
		wantQuota  int64
		wantPeriod int64
	}{
		{
			name:       "long flag half core 0.5",
			args:       []string{"--cpus", "0.5", "/bin/sh"},
			wantQuota:  50000,
			wantPeriod: 100000,
		},
		{
			name:       "long flag equal-separated 1.0",
			args:       []string{"--cpus=1.0", "/bin/sh"},
			wantQuota:  100000,
			wantPeriod: 100000,
		},
		{
			name:       "short flag space-separated 2.0",
			args:       []string{"-c", "2.0", "/bin/sh"},
			wantQuota:  200000,
			wantPeriod: 100000,
		},
		{
			name:       "short flag equal-separated 0.25",
			args:       []string{"-c=0.25", "/bin/sh"},
			wantQuota:  25000,
			wantPeriod: 100000,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedCfg container.Config
			cmd := newRunCmd(func(cfg container.Config) error {
				capturedCfg = cfg
				return nil
			})
			cmd.SetArgs(tc.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if capturedCfg.CpuQuota != tc.wantQuota {
				t.Errorf("expected CpuQuota %d, got %d", tc.wantQuota, capturedCfg.CpuQuota)
			}
			if capturedCfg.CpuPeriod != tc.wantPeriod {
				t.Errorf("expected CpuPeriod %d, got %d", tc.wantPeriod, capturedCfg.CpuPeriod)
			}
		})
	}
}

func TestNewRunCmd_CombinedFlagsAndSubcommandArgs(t *testing.T) {
	var capturedCfg container.Config
	mockRunner := func(cfg container.Config) error {
		capturedCfg = cfg
		return nil
	}

	cmd := newRunCmd(mockRunner)
	// Notice: container command has "-c" which should not be confused with minicontainer --cpus/-c flag!
	cmd.SetArgs([]string{
		"--memory", "100m",
		"--pids", "20",
		"--cpus", "0.5",
		"--rootfs", "/custom/rootfs",
		"/bin/sh", "-c", "echo hello",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedCfg.MemoryLimit != 104857600 {
		t.Errorf("expected MemoryLimit 104857600, got %d", capturedCfg.MemoryLimit)
	}
	if capturedCfg.PidsLimit != 20 {
		t.Errorf("expected PidsLimit 20, got %d", capturedCfg.PidsLimit)
	}
	if capturedCfg.CpuQuota != 50000 {
		t.Errorf("expected CpuQuota 50000, got %d", capturedCfg.CpuQuota)
	}
	if capturedCfg.CpuPeriod != 100000 {
		t.Errorf("expected CpuPeriod 100000, got %d", capturedCfg.CpuPeriod)
	}
	if capturedCfg.Rootfs != "/custom/rootfs" {
		t.Errorf("expected Rootfs /custom/rootfs, got %q", capturedCfg.Rootfs)
	}
	if capturedCfg.Command != "/bin/sh" {
		t.Errorf("expected Command /bin/sh, got %q", capturedCfg.Command)
	}
	if len(capturedCfg.Args) != 2 || capturedCfg.Args[0] != "-c" || capturedCfg.Args[1] != "echo hello" {
		t.Errorf("expected Args ['-c', 'echo hello'], got %v", capturedCfg.Args)
	}
}

func TestNewRunCmd_InvalidFlagValues(t *testing.T) {
	testCases := []struct {
		name string
		args []string
	}{
		{name: "missing memory argument", args: []string{"--memory"}},
		{name: "missing memory short argument", args: []string{"-m"}},
		{name: "invalid memory value", args: []string{"--memory", "100invalid", "/bin/sh"}},
		{name: "negative memory value", args: []string{"--memory", "-50m", "/bin/sh"}},
		{name: "missing pids argument", args: []string{"--pids"}},
		{name: "missing pids short argument", args: []string{"-p"}},
		{name: "invalid pids value", args: []string{"--pids", "invalid", "/bin/sh"}},
		{name: "negative pids value", args: []string{"--pids", "-10", "/bin/sh"}},
		{name: "missing cpus argument", args: []string{"--cpus"}},
		{name: "missing cpus short argument", args: []string{"-c"}},
		{name: "invalid cpus value", args: []string{"--cpus", "invalid", "/bin/sh"}},
		{name: "negative cpus value", args: []string{"--cpus", "-1.5", "/bin/sh"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newRunCmd(func(cfg container.Config) error { return nil })
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error for args %v, got nil", tc.args)
			}
		})
	}
}

