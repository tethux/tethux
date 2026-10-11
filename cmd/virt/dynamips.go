package virt

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	native "github.com/tethux/dynamips-downgrade"
	"github.com/tethux/dynamips-downgrade/console"
	"github.com/tethux/tethux/storage"
	localstorage "github.com/tethux/tethux/storage/local"
	virtmodel "github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/emulator"
	"github.com/tethux/tethux/virt/emulator/dynamips"
	"github.com/tethux/tethux/virt/emulator/dynamips/errs"
)

func dynamipsCmd() *cobra.Command {
	root := &cobra.Command{Use: "dynamips", Short: "Run embedded routers with persistent state"}
	var image, name, platform, stateRoot, startup string
	var ram, port uint32
	var enhanced, attach bool
	var duration time.Duration
	run := &cobra.Command{
		Use: "run", Short: "Boot a router in a foreground session", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) (resultErr error) {
			if image == "" || ram == 0 || ram > 2048 || port == 0 || port > 65535 {
				return errs.New("run router", errs.ErrConfig, "image, RAM in 1..2048 MiB, and console port in 1..65535 are required")
			}
			absoluteImage, imageErr := filepath.Abs(image)
			if imageErr != nil {
				return imageErr
			}
			store, storageErr := localstorage.New(stateRoot)
			if storageErr != nil {
				return storageErr
			}
			manager := dynamips.NewManager(store)
			cfg := &dynamips.Config{
				Config:   emulator.Config{NodeConfig: virtmodel.NodeConfig{ID: name, Name: name, Image: absoluteImage, MemoryMB: int(ram)}},
				State:    storage.Ref{Provider: store.Name(), Key: storage.Key(name)},
				Platform: dynamips.PlatformConfig{Name: platform}, InstanceID: 1, ConsolePort: uint16(port), EnhancedConsole: enhanced,
			}
			if platform == "c7200" {
				cfg.Platform.C7200 = &dynamips.C7200Config{NPE: "npe-400", Midplane: "vxr"}
			}
			if startup != "" {
				data, readErr := os.ReadFile(startup) // #nosec G304 -- Explicit operator-selected provisioning file.
				if readErr != nil {
					return readErr
				}
				cfg.StartupConfig, cfg.PrivateConfig = data, []byte{}
			}
			ctx, stop := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if duration > 0 {
				timed, cancel := context.WithTimeout(ctx, duration)
				defer cancel()
				ctx = timed
			}
			runtime, runtimeErr := native.New()
			if runtimeErr != nil {
				return runtimeErr
			}
			adapter := &dynamips.Adapter{Runtime: runtime}
			var resources *dynamips.PreparedResources
			defer func() {
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
				defer cancel()
				resultErr = errors.Join(resultErr, manager.Delete(cleanupCtx, adapter, name, resources), adapter.Close(), runtime.Close())
			}()
			_, prepared, createErr := manager.Create(ctx, adapter, cfg)
			resources = prepared
			if createErr != nil {
				return createErr
			}
			if startErr := adapter.Start(ctx, name); startErr != nil {
				return startErr
			}
			command.Printf("%s running; console 127.0.0.1:%d; state %s\n", name, port, resources.State.Location.Value)
			if enhanced {
				raw, consoleErr := console.RunEnhanced(ctx, cfg.ConsolePort)
				if consoleErr != nil || !raw {
					return consoleErr
				}
				attach = true
			}
			if attach {
				client := exec.CommandContext(ctx, "telnet", "127.0.0.1", strconv.Itoa(int(port))) // #nosec G204 -- Fixed executable and loopback host; the numeric port is range-checked.
				client.Stdin, client.Stdout, client.Stderr = os.Stdin, command.OutOrStdout(), command.ErrOrStderr()
				clientErr := client.Run()
				if ctx.Err() != nil {
					return nil
				}
				return clientErr
			}
			<-ctx.Done()
			return nil
		},
	}
	flags := run.Flags()
	flags.StringVar(&image, "image", os.Getenv("TETHUX_DYNAMIPS_IMAGE"), "local IOS image path")
	flags.StringVar(&name, "name", "router", "router identity and state directory key")
	flags.StringVar(&platform, "platform", "c7200", "Dynamips platform")
	flags.StringVar(&stateRoot, "state-root", filepath.Join(os.Getenv("HOME"), ".local", "share", "tethux", "routers"), "persistent router state root")
	flags.StringVar(&startup, "startup-config", "", "explicitly provision NVRAM from this file; omitted preserves saved state")
	flags.Uint32Var(&ram, "ram", 512, "router RAM in MiB")
	flags.Uint32Var(&port, "console-port", 20000, "TCP console port")
	flags.BoolVar(&enhanced, "enhanced", false, "use local console editing and history")
	flags.BoolVar(&attach, "console", true, "attach a Telnet console; false runs until Ctrl+C")
	flags.DurationVar(&duration, "duration", 0, "stop after this duration; zero waits for Ctrl+C")
	root.AddCommand(run)
	return root
}
