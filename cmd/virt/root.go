package virt

import (
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "virt",
		Short: "tethux virt - container/vm provider smoke tests",
	}

	cmd.AddCommand(
		smokeCmd(),
		listCmd(),
		pullCmd(),
		logsCmd(),
	)

	return cmd
}
