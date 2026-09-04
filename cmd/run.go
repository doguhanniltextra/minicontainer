package cmd

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"minicontainer/internal/container"
)

const (
	defaultHostname  = "minicontainer"
	defaultRootfs    = "assets/rootfs"
	defaultCpuPeriod = 100000 // 100ms in microseconds
)

// runOptions holds the parsed flag configuration for a container run.
type runOptions struct {
	rootfs      string
	memoryLimit int64
	pidsLimit   int64
	cpuQuota    int64
	cpuPeriod   int64
}

// containerRunner abstracts executing the container logic from the CLI command.
type containerRunner func(cfg container.Config) error

// rootCmd is the base command for minicontainer.
var rootCmd = &cobra.Command{
	Use:   "minicontainer",
	Short: "A minimal container runtime",
}

// parseMemory parses a human-readable memory limit string (e.g. "100m", "1g", "512M")
// into bytes. An empty string or "0" returns 0 (unlimited).
func parseMemory(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}

	lastChar := s[len(s)-1]
	var multiplier int64 = 1
	numStr := s

	switch lastChar {
	case 'b', 'B':
		multiplier = 1
		numStr = s[:len(s)-1]
	case 'k', 'K':
		multiplier = 1024
		numStr = s[:len(s)-1]
	case 'm', 'M':
		multiplier = 1024 * 1024
		numStr = s[:len(s)-1]
	case 'g', 'G':
		multiplier = 1024 * 1024 * 1024
		numStr = s[:len(s)-1]
	case 't', 'T':
		multiplier = 1024 * 1024 * 1024 * 1024
		numStr = s[:len(s)-1]
	default:
		if lastChar < '0' || lastChar > '9' {
			return 0, fmt.Errorf("invalid memory limit unit in %q", s)
		}
	}

	if numStr == "" {
		return 0, fmt.Errorf("invalid memory limit: %q", s)
	}

	val, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory limit %q: %w", s, err)
	}
	if val < 0 {
		return 0, fmt.Errorf("memory limit must be non-negative: %q", s)
	}

	return val * multiplier, nil
}

// extractFlagValue checks if the current argument matches the given long or short flag,
// supporting both space-separated ("--flag", "val") and equal-separated ("--flag=val") forms.
func extractFlagValue(args []string, i *int, long, short string) (string, bool, error) {
	arg := args[*i]
	if arg == long || arg == short {
		if *i+1 >= len(args) {
			return "", true, fmt.Errorf("flag needs an argument: %s", arg)
		}
		val := args[*i+1]
		*i += 2
		return val, true, nil
	}
	if strings.HasPrefix(arg, long+"=") {
		val := strings.TrimPrefix(arg, long+"=")
		*i++
		return val, true, nil
	}
	if strings.HasPrefix(arg, short+"=") {
		val := strings.TrimPrefix(arg, short+"=")
		*i++
		return val, true, nil
	}
	return "", false, nil
}

// parseRunArgs parses custom CLI flags (such as --rootfs, --memory, --pids, --cpus) while
// treating all subsequent arguments as the container command and arguments.
func parseRunArgs(args []string) (runOptions, []string, error) {
	opts := runOptions{
		rootfs:    defaultRootfs,
		cpuPeriod: defaultCpuPeriod,
	}
	i := 0

	for i < len(args) {
		if val, matched, err := extractFlagValue(args, &i, "--rootfs", "-r"); matched {
			if err != nil {
				return opts, nil, err
			}
			opts.rootfs = val
			continue
		}

		if val, matched, err := extractFlagValue(args, &i, "--memory", "-m"); matched {
			if err != nil {
				return opts, nil, err
			}
			mem, err := parseMemory(val)
			if err != nil {
				return opts, nil, err
			}
			opts.memoryLimit = mem
			continue
		}

		if val, matched, err := extractFlagValue(args, &i, "--pids", "-p"); matched {
			if err != nil {
				return opts, nil, err
			}
			pids, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return opts, nil, fmt.Errorf("invalid pids limit %q: %w", val, err)
			}
			if pids < 0 {
				return opts, nil, fmt.Errorf("pids limit must be non-negative: %d", pids)
			}
			opts.pidsLimit = pids
			continue
		}

		if val, matched, err := extractFlagValue(args, &i, "--cpus", "-c"); matched {
			if err != nil {
				return opts, nil, err
			}
			cpus, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return opts, nil, fmt.Errorf("invalid cpus limit %q: %w", val, err)
			}
			if cpus < 0 {
				return opts, nil, fmt.Errorf("cpus limit must be non-negative: %v", cpus)
			}
			opts.cpuQuota = int64(cpus * float64(defaultCpuPeriod))
			continue
		}

		// First non-flag argument marks the start of the container command
		break
	}

	remaining := args[i:]
	if len(remaining) == 0 {
		return opts, nil, errors.New("requires at least 1 arg(s), only received 0")
	}

	return opts, remaining, nil
}

// newRunCmd constructs the `minicontainer run` subcommand with the provided runner.
func newRunCmd(runner containerRunner) *cobra.Command {
	return &cobra.Command{
		Use:                "run [--rootfs <path>] [-m|--memory <limit>] [-p|--pids <limit>] [-c|--cpus <cores>] <command> [args...]",
		Short:              "Start a new container",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				return cmd.Help()
			}

			opts, cmdArgs, err := parseRunArgs(args)
			if err != nil {
				return err
			}

			cfg := container.Config{
				Command:     cmdArgs[0],
				Args:        cmdArgs[1:],
				Hostname:    defaultHostname,
				Rootfs:      opts.rootfs,
				MemoryLimit: opts.memoryLimit,
				PidsLimit:   opts.pidsLimit,
				CpuQuota:    opts.cpuQuota,
				CpuPeriod:   opts.cpuPeriod,
			}
			return runner(cfg)
		},
	}
}

// Execute is called by main.go to run the root CLI command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// When the binary is re-executed as the container child, os.Args[1] will be
	// ContainerInitArg. Detect this before cobra processes arguments and route
	// directly to container.Init(), which runs inside the new namespaces.
	if len(os.Args) > 1 && os.Args[1] == container.ContainerInitArg {
		if err := container.Init(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	rootCmd.AddCommand(newRunCmd(container.Run))
}
