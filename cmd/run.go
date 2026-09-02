package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"minicontainer/internal/container"
)

const (
	defaultHostname = "minicontainer"
	defaultRootfs   = "assets/rootfs"
)

// containerRunner abstracts executing the container logic from the CLI command.
type containerRunner func(cfg container.Config) error

// rootCmd is the base command for minicontainer.
var rootCmd = &cobra.Command{
	Use:   "minicontainer",
	Short: "A minimal container runtime",
}

// parseRunArgs parses custom CLI flags (such as --rootfs or -r) while treating
// all subsequent arguments as the container command and arguments.
func parseRunArgs(args []string) (string, []string, error) {
	rootfs := defaultRootfs
	i := 0

	for i < len(args) {
		arg := args[i]
		if arg == "--rootfs" || arg == "-r" {
			if i+1 >= len(args) {
				return "", nil, errors.New("flag needs an argument: " + arg)
			}
			rootfs = args[i+1]
			i += 2
		} else if strings.HasPrefix(arg, "--rootfs=") {
			rootfs = strings.TrimPrefix(arg, "--rootfs=")
			i++
		} else if strings.HasPrefix(arg, "-r=") {
			rootfs = strings.TrimPrefix(arg, "-r=")
			i++
		} else {
			// First non-flag argument marks the start of the container command
			break
		}
	}

	remaining := args[i:]
	if len(remaining) == 0 {
		return "", nil, errors.New("requires at least 1 arg(s), only received 0")
	}

	return rootfs, remaining, nil
}

// newRunCmd constructs the `minicontainer run` subcommand with the provided runner.
func newRunCmd(runner containerRunner) *cobra.Command {
	return &cobra.Command{
		Use:                "run [--rootfs <path>] <command> [args...]",
		Short:              "Start a new container",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				return cmd.Help()
			}

			rootfs, cmdArgs, err := parseRunArgs(args)
			if err != nil {
				return err
			}

			cfg := container.Config{
				Command:  cmdArgs[0],
				Args:     cmdArgs[1:],
				Hostname: defaultHostname,
				Rootfs:   rootfs,
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
