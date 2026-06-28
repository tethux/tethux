package main

import (
	"fmt"
	"os"

	"github.com/0xveya/tethux/cmd/bridge"
)

func main() {
	if err := bridge.NewRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "tethux-bridge: %v\n", err)
		os.Exit(1)
	}
}
