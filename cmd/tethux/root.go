package main

import "github.com/spf13/cobra"

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tethux",
		Short: "CLI for testing the tethux switch library",
	}

	cmd.AddCommand(newBridgeCmd())
	cmd.AddCommand(newFrameCmd())

	return cmd
}
