package bridge

import "github.com/spf13/cobra"

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bridge",
		Short: "CLI for testing the tethux switch library",
	}

	cmd.AddCommand(newBridgeCmd())
	cmd.AddCommand(newFrameCmd())

	return cmd
}
