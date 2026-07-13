package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/0xveya/tethux/cmd/virt"
)

func init() {
	if runtime.GOOS == "windows" {
		panic("not supported os")
	}
}

func main() {
	if err := virt.NewRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "tethux-virt: %v\n", err)
		os.Exit(1)
	}
}
