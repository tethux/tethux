package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/0xveya/tethux/cmd/bridge"
)

func init() {
	if runtime.GOOS == "windows" {
		panic("not supported os")
	}
}

func main() {
	if err := bridge.NewRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "tethux-bridge: %v\n", err)
		os.Exit(1)
	}
}
