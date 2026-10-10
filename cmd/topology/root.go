// Package topology provides commands for running TOML and Lua topologies.
package topology

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	topologymodel "github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
	"github.com/tethux/tethux/topology/local"
	topologylua "github.com/tethux/tethux/topology/lua"
	topologytoml "github.com/tethux/tethux/topology/toml"
)

// NewRootCmd creates the topology command tree.
func NewRootCmd() *cobra.Command {
	command := &cobra.Command{Use: "topology", Short: "Run network topologies from TOML or Lua", SilenceErrors: true}
	command.AddCommand(newRunCmd())
	return command
}

func newRunCmd() *cobra.Command {
	var providerName string
	var basePort int
	command := &cobra.Command{
		Use: "run <file.toml|file.lua>", Short: "Run a local container topology until Ctrl+C",
		Long: "Run a local container topology until Ctrl+C. Files ending in .lua execute trusted Lua scripts; other files are decoded as TOML.",
		Args: cobra.ExactArgs(1), SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) (resultErr error) {
			top, err := readTopology(args[0])
			if err != nil {
				return err
			}
			if os.Geteuid() != 0 {
				return errs.New("run topology", errs.ErrStart, "namespace and raw-socket privileges required")
			}
			ctx, stop := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			provider, err := local.SelectContainerProvider(ctx, providerName)
			if err != nil {
				return err
			}
			logger := slog.New(slog.NewJSONHandler(command.ErrOrStderr(), nil)).With("topology", string(top.ID))
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
			err = logSummary(ctx, logger, top, running, provider)
			if err != nil {
				return err
			}
			logger.InfoContext(ctx, "Topology ready", "stop", "Ctrl+C")
			<-ctx.Done()
			return nil
		},
	}
	command.Flags().StringVar(&providerName, "provider", "auto", "container provider: auto, docker, podman, or containerd")
	command.Flags().IntVar(&basePort, "base-port", 23000, "first loopback UDP link port")
	return command
}

func readTopology(path string) (*topologymodel.Topology, error) {
	file, err := os.Open(path) // #nosec G304 -- The CLI intentionally reads the topology path supplied by the user.
	if err != nil {
		return nil, errs.Wrap("open topology", errs.ErrDecode, path, err)
	}
	var top *topologymodel.Topology
	if strings.EqualFold(filepath.Ext(path), ".lua") {
		top, err = topologylua.Decode(file)
	} else {
		top, err = topologytoml.Decode(file)
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return nil, errs.Wrap("read topology", errs.ErrDecode, path, errors.Join(err, closeErr))
	}
	return top, nil
}
