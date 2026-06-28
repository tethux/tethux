package main

import (
	"fmt"
	"os"

	"github.com/0xveya/tethux/cmd/virt"
)

func main() {
	if err := virt.NewRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "tethux-virt: %v\n", err)
		os.Exit(1)
	}
}
