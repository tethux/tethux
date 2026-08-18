package virt

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/domain"
	libvirtprovider "github.com/tethux/tethux/virt/hypervisor/libvirt"
)

func libvirtCmd() *cobra.Command {
	var uri, name, disk, action string
	var keep bool

	command := &cobra.Command{
		Use:   "libvirt",
		Short: "create and exercise one local libvirt domain",
		RunE: func(command *cobra.Command, _ []string) error {
			ctx := command.Context()
			provider, connectErr := libvirtprovider.New(uri)
			if connectErr != nil {
				return connectErr
			}
			defer provider.Close()

			if action != "create" {
				return runLibvirtAction(ctx, provider, uri, name, action)
			}
			if disk == "" {
				return fmt.Errorf("--disk is required when --action=create")
			}
			absoluteDisk, pathErr := filepath.Abs(disk)
			if pathErr != nil {
				return fmt.Errorf("resolve disk path: %w", pathErr)
			}

			config := &domain.RuntimeConfig{
				NodeConfig:   domainNodeConfig(name),
				Architecture: "x86_64",
				Machine:      "q35",
				Disks:        []domain.RuntimeDisk{{Source: absoluteDisk, Bus: "virtio", Target: "vda", Format: "qcow2"}},
			}
			node, createErr := provider.CreateDomain(ctx, config)
			if createErr != nil {
				return createErr
			}
			fmt.Printf("created %s (%s) state=%s\n", node.Name, node.ID, node.State)
			if keep {
				return nil
			}
			stopErr := provider.Stop(ctx, node.ID)
			if stopErr != nil {
				return stopErr
			}
			deleteErr := provider.Delete(ctx, node.ID)
			if deleteErr != nil {
				return deleteErr
			}
			fmt.Println("stopped and deleted")
			return nil
		},
	}
	command.Flags().StringVar(&uri, "uri", "qemu:///system", "libvirt connection URI")
	command.Flags().StringVar(&name, "name", "tethux-libvirt-test", "domain name")
	command.Flags().StringVar(&disk, "disk", "", "path to a local qcow2 or raw disk")
	command.Flags().StringVar(&action, "action", "create", "action: create, list, start, stop, poweroff, suspend, resume, restart, delete, view")
	command.Flags().BoolVar(&keep, "keep", false, "leave the domain running after create")
	return command
}

func runLibvirtAction(ctx context.Context, provider *libvirtprovider.Provider, uri, name, action string) error {
	var actionErr error
	switch action {
	case "start":
		actionErr = provider.Start(ctx, name)
	case "stop":
		actionErr = provider.Stop(ctx, name)
	case "poweroff":
		actionErr = provider.PowerOff(ctx, name)
	case "suspend":
		actionErr = provider.Suspend(ctx, name)
	case "resume":
		actionErr = provider.Resume(ctx, name)
	case "restart":
		actionErr = provider.Restart(ctx, name)
	case "delete":
		actionErr = provider.Delete(ctx, name)
	case "list":
		nodes, listErr := provider.List(ctx)
		if listErr != nil {
			return listErr
		}
		for _, node := range nodes {
			fmt.Printf("%-36s %-24s %s\n", node.ID, node.Name, node.State)
		}
		return nil
	case "view":
		viewer := osexec.CommandContext(ctx, "virt-viewer", "--connect", uri, name) // #nosec G204 -- fixed viewer and operator-selected libvirt name.
		viewer.Stdin = os.Stdin
		viewer.Stdout = os.Stdout
		viewer.Stderr = os.Stderr
		actionErr = viewer.Run()
	default:
		return fmt.Errorf("unknown --action %q", action)
	}
	return actionErr
}

func domainNodeConfig(name string) (config virt.NodeConfig) {
	return virt.NodeConfig{Name: name, CPUs: 1, MemoryMB: 1024}
}
