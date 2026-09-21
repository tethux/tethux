// Package topology provides commands for running declarative TOML topologies.
package topology

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tethux/tethux/topology/errs"
	"github.com/tethux/tethux/topology/local"
	topologytoml "github.com/tethux/tethux/topology/toml"
)

// NewRootCmd creates the topology command tree.
func NewRootCmd() *cobra.Command {
	command := &cobra.Command{Use: "topology", Short: "Run declarative network topologies", SilenceErrors: true}
	command.AddCommand(newRunCmd())
	return command
}

func newRunCmd() *cobra.Command {
	var providerName string
	var basePort int
	command := &cobra.Command{
		Use: "run <file.toml>", Short: "Run a local container topology until Ctrl+C",
		Args: cobra.ExactArgs(1), SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) (resultErr error) {
			file, err := os.Open(args[0])
			if err != nil {
				return errs.Wrap("open topology TOML", errs.ErrDecode, args[0], err)
			}
			top, err := topologytoml.Decode(file)
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				return errors.Join(err, closeErr)
			}
			if os.Geteuid() != 0 {
				executable, executableErr := os.Executable()
				if executableErr != nil {
					return executableErr
				}
				absolutePath, pathErr := filepath.Abs(args[0])
				if pathErr != nil {
					return pathErr
				}
				parts := []string{"pkexec", executable, "topology", "run", absolutePath, "--provider", providerName, "--base-port", fmt.Sprint(basePort)}
				for index, part := range parts {
					parts[index] = "'" + strings.ReplaceAll(part, "'", "'\\''") + "'"
				}
				return errs.New("run topology", errs.ErrStart, "namespace and raw-socket privileges required; run: "+strings.Join(parts, " "))
			}
			ctx, stop := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			provider, err := local.SelectContainerProvider(ctx, providerName)
			if err != nil {
				return err
			}
			logger := slog.New(slog.NewTextHandler(command.ErrOrStderr(), nil)).With("topology", string(top.ID))
			logger.InfoContext(ctx, "Starting topology", "provider", provider.Info().Name, "nodes", len(top.Nodes), "links", len(top.Links))
			running, err := local.Start(ctx, top, local.Options{Provider: provider, BasePort: basePort, Logger: logger})
			if err != nil {
				return err
			}
			defer func() {
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
				defer cancel()
				logger.InfoContext(cleanupCtx, "Stopping topology")
				cleanupErr := running.Close(cleanupCtx)
				resultErr = errors.Join(resultErr, cleanupErr)
				if cleanupErr == nil {
					logger.InfoContext(cleanupCtx, "Topology stopped; resources removed")
				}
			}()
			err = printSummary(ctx, command.OutOrStdout(), top, running, provider)
			if err != nil {
				return err
			}
			logger.InfoContext(ctx, "Topology ready", "stop", "Ctrl+C")
			<-ctx.Done()
			return nil
		},
	}
	command.Flags().StringVar(&providerName, "provider", "auto", "container provider: auto, docker, or podman")
	command.Flags().IntVar(&basePort, "base-port", 23000, "first loopback UDP link port")
	return command
}
