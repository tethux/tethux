package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/0xveya/tethux/cmd/bridge"
	"github.com/0xveya/tethux/cmd/virt"
)

func init() {
	if runtime.GOOS == "windows" {
		panic("not supported os")
	}
}

func main() {
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
		if err := newRootCmd().Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "tethux: %v\n", err)
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
