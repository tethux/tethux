package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"

	"github.com/tethux/tethux/cmd/bridge"
	"github.com/tethux/tethux/cmd/virt"
)

func init() {
	if runtime.GOOS == "windows" {
		panic("not supported os")
	}
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	switch argv0() {
	case "bridge":
		if err := bridge.NewRootCmd().Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "tethux-bridge: %v\n", err)
			os.Exit(1)
		}
	case "virt":
		if err := virt.NewRootCmd().Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "tethux-virt: %v\n", err)
			os.Exit(1)
		}
	case "tethux":
		command, err := newRootCmd().ExecuteC()
		if err != nil {
			logger := slog.Default()
			if command != nil && (command.CommandPath() == "tethux topology" || strings.HasPrefix(command.CommandPath(), "tethux topology ")) {
				logger = slog.New(slog.NewJSONHandler(os.Stderr, nil))
			}
			logger.Error("Command failed", "error", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "tethux: unknown command %q\n", argv0())
		os.Exit(1)
	}
}

func argv0() string {
	arg := os.Args[0]
	parts := strings.Split(arg, "/")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return os.Args[0]
}
