package main

import (
	"github.com/spf13/cobra"

	"github.com/0xveya/tethux/cmd/bridge"
	"github.com/0xveya/tethux/cmd/virt"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tethux",
		Short: "tethux cli for all components",
		Long: `A multicall CLI binary that bundles all tethux components. 
Acts as a standard subcommand suite, or dispatches directly to a 
component when invoked via a matching symlink (argv[0]).`,
	}

	cmd.AddCommand(virt.NewRootCmd(), bridge.NewRootCmd())

	return cmd
}
