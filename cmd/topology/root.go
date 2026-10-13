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

	"github.com/tethux/tethux/internal/cleanup"

	"github.com/spf13/cobra"
	native "github.com/tethux/dynamips-downgrade"
	topologymodel "github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
	"github.com/tethux/tethux/topology/local"
	topologylua "github.com/tethux/tethux/topology/lua"
	topologytoml "github.com/tethux/tethux/topology/toml"
	"github.com/tethux/tethux/virt/container"
	"github.com/tethux/tethux/virt/emulator/dynamips"
)

// NewRootCmd creates the topology command tree.
func NewRootCmd() *cobra.Command {
	command := &cobra.Command{Use: "topology", Short: "Run network topologies from TOML or Lua", SilenceErrors: true}
	command.AddCommand(newRunCmd())
	return command
}

func newRunCmd() *cobra.Command {
	var providerName, stateRoot, dynamipsImage string
	var basePort, consoleBasePort int
	var duration time.Duration
	command := &cobra.Command{
		Use: "run <file.toml|file.lua>", Short: "Run a local container or Dynamips topology until Ctrl+C",
		Long: "Run a local topology until Ctrl+C. Files ending in .lua execute trusted Lua scripts; other files are decoded as TOML.",
		Args: cobra.ExactArgs(1), SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) (resultErr error) {
			top, decodeErr := readTopology(args[0])
			if decodeErr != nil {
				return decodeErr
			}
			var hasContainers, hasRouters bool
			for _, node := range top.Nodes {
				hasContainers = hasContainers || node.Spec.Kind() == topologymodel.NodeKindContainer
				hasRouters = hasRouters || node.Spec.Kind() == topologymodel.NodeKindDynamips
			}
			if hasContainers && os.Geteuid() != 0 {
				return errs.New("run topology", errs.ErrStart, "namespace and raw-socket privileges required")
			}
			ctx, stop := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if duration > 0 {
				timed, cancel := context.WithTimeout(ctx, duration)
				defer cancel()
				ctx = timed
			}
			var provider container.ContainerProvider
			if hasContainers {
				selected, providerErr := local.SelectContainerProvider(ctx, providerName)
				if providerErr != nil {
					return providerErr
				}
				provider = selected
			}
			var routers *dynamips.Adapter
			if hasRouters {
				runtime, runtimeErr := native.New()
				if runtimeErr != nil {
					return runtimeErr
				}
				routers = &dynamips.Adapter{Runtime: runtime}
				defer func() { resultErr = errors.Join(resultErr, routers.Close(), runtime.Close()) }()
			}
			logger := slog.New(slog.NewJSONHandler(command.ErrOrStderr(), nil)).With("topology", string(top.ID))
			logger.InfoContext(ctx, "Starting topology", "nodes", len(top.Nodes), "links", len(top.Links))
			running, startErr := local.Start(ctx, top, local.Options{
				Provider: provider, BasePort: basePort, Logger: logger,
				Dynamips: routerOptions(routers, stateRoot, dynamipsImage, consoleBasePort),
			})
			if startErr != nil {
				if running != nil {
					cleanupCtx, cancel := cleanup.NewContext(ctx)
					defer cancel()
					return errors.Join(startErr, running.Close(cleanupCtx))
				}
				return startErr
			}
			defer func() {
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
				defer cancel()
				logger.InfoContext(cleanupCtx, "Stopping topology")
				cleanupErr := running.Close(cleanupCtx)
				resultErr = errors.Join(resultErr, cleanupErr)
				if cleanupErr == nil {
					logger.InfoContext(cleanupCtx, "Topology stopped; resources released")
				}
			}()
			if summaryErr := logSummary(ctx, logger, top, running, provider, routers); summaryErr != nil {
				return summaryErr
			}
			logger.InfoContext(ctx, "Topology ready", "stop", "Ctrl+C")
			<-ctx.Done()
			return nil
		},
	}
	command.Flags().StringVar(&providerName, "provider", "auto", "container provider: auto, docker, podman, or containerd")
	command.Flags().IntVar(&basePort, "base-port", 23000, "first loopback UDP link port")
	command.Flags().StringVar(&stateRoot, "state-root", filepath.Join(os.Getenv("HOME"), ".local", "share", "tethux", "routers"), "persistent router state root")
	command.Flags().StringVar(&dynamipsImage, "dynamips-image", os.Getenv("TETHUX_DYNAMIPS_IMAGE"), "override local IOS image paths")
	command.Flags().IntVar(&consoleBasePort, "console-base-port", 20000, "first TCP router console port")
	command.Flags().DurationVar(&duration, "duration", 0, "stop after this duration; zero waits for Ctrl+C")
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

func routerOptions(provider *dynamips.Adapter, stateRoot, image string, consoleBasePort int) *local.DynamipsOptions {
	if provider == nil {
		return nil
	}
	return &local.DynamipsOptions{Provider: provider, StateRoot: stateRoot, Image: image, ConsoleBasePort: consoleBasePort}
}
