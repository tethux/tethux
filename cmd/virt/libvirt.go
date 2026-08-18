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

const (
	defaultLibvirtURI   = "qemu:///system"
	defaultDomainName   = "tethux-libvirt-test"
	defaultArchitecture = "x86_64"
	defaultMachine      = "q35"
)

func libvirtCmd() *cobra.Command {
	var (
		uri    string
		name   string
		id     string
		disk   string
		action string
		keep   bool
	)

	command := &cobra.Command{
		Use:   "libvirt",
		Short: "create and manage local libvirt domains",
		RunE: func(command *cobra.Command, _ []string) error {
			ctx := command.Context()

			provider, connectErr := libvirtprovider.New(uri)
			if connectErr != nil {
				return connectErr
			}
			defer provider.Close()

			switch action {
			case "create":
				return createLibvirtDomain(ctx, provider, name, disk, keep)

			case "list":
				return listLibvirtDomains(ctx, provider)

			case "start",
				"stop",
				"poweroff",
				"suspend",
				"resume",
				"restart",
				"delete",
				"view":

				if id == "" {
					return fmt.Errorf(
						"--id is required when --action=%s",
						action,
					)
				}

				return runLibvirtAction(
					ctx,
					provider,
					uri,
					id,
					action,
				)

			default:
				return fmt.Errorf("unknown --action %q", action)
			}
		},
	}

	flags := command.Flags()

	flags.StringVar(
		&uri,
		"uri",
		defaultLibvirtURI,
		"libvirt connection URI",
	)

	flags.StringVar(
		&name,
		"name",
		defaultDomainName,
		"domain name when creating",
	)

	flags.StringVar(
		&id,
		"id",
		"",
		"domain ID",
	)

	flags.StringVar(
		&disk,
		"disk",
		"",
		"path to a local qcow2 or raw disk",
	)

	flags.StringVar(
		&action,
		"action",
		"create",
		"action: create, list, start, stop, poweroff, suspend, resume, restart, delete, view",
	)

	flags.BoolVar(
		&keep,
		"keep",
		false,
		"leave the domain running after create",
	)

	return command
}

func createLibvirtDomain(
	ctx context.Context,
	provider *libvirtprovider.Provider,
	name string,
	disk string,
	keep bool,
) error {
	if disk == "" {
		return fmt.Errorf("--disk is required when --action=create")
	}

	absoluteDisk, pathErr := filepath.Abs(disk)
	if pathErr != nil {
		return fmt.Errorf("resolve disk path: %w", pathErr)
	}

	config := &domain.RuntimeConfig{
		NodeConfig: domainNodeConfig(name),

		Architecture: defaultArchitecture,
		Machine:      defaultMachine,

		Disks: []domain.RuntimeDisk{
			{
				Source: absoluteDisk,
				Bus:    string(domain.DiskBusVirtio),
				Target: "vda",
				Format: string(domain.DiskFormatQCOW2),
			},
		},
	}

	node, createErr := provider.CreateDomain(ctx, config)
	if createErr != nil {
		return createErr
	}

	fmt.Printf(
		"created %s\n  id:    %s\n  state: %s\n",
		node.Name,
		node.ID,
		node.State,
	)

	if keep {
		return nil
	}

	if stopErr := provider.Stop(ctx, node.ID); stopErr != nil {
		return stopErr
	}

	if deleteErr := provider.Delete(ctx, node.ID); deleteErr != nil {
		return deleteErr
	}

	fmt.Println("stopped and deleted")
	return nil
}

func listLibvirtDomains(
	ctx context.Context,
	provider *libvirtprovider.Provider,
) error {
	nodes, listErr := provider.List(ctx)
	if listErr != nil {
		return listErr
	}

	if len(nodes) == 0 {
		fmt.Println("no managed domains")
		return nil
	}

	fmt.Printf("%-36s  %-24s  %s\n", "ID", "NAME", "STATE")

	for _, node := range nodes {
		fmt.Printf(
			"%-36s  %-24s  %s\n",
			node.ID,
			node.Name,
			node.State,
		)
	}

	return nil
}

func runLibvirtAction(
	ctx context.Context,
	provider *libvirtprovider.Provider,
	uri string,
	id string,
	action string,
) error {
	switch action {
	case "start":
		return provider.Start(ctx, id)

	case "stop":
		return provider.Stop(ctx, id)

	case "poweroff":
		return provider.PowerOff(ctx, id)

	case "suspend":
		return provider.Suspend(ctx, id)

	case "resume":
		return provider.Resume(ctx, id)

	case "restart":
		return provider.Restart(ctx, id)

	case "delete":
		return provider.Delete(ctx, id)

	case "view":
		return viewLibvirtDomain(ctx, provider, uri, id)

	default:
		return fmt.Errorf("unsupported domain action %q", action)
	}
}

func viewLibvirtDomain(
	ctx context.Context,
	provider *libvirtprovider.Provider,
	uri string,
	id string,
) error {
	node, inspectErr := provider.InspectDomain(ctx, id)
	if inspectErr != nil {
		return inspectErr
	}

	viewer := osexec.CommandContext(
		ctx,
		"virt-viewer",
		"--connect",
		uri,
		node.Name,
	) // #nosec G204 -- fixed executable with operator-selected libvirt connection.

	viewer.Stdin = os.Stdin
	viewer.Stdout = os.Stdout
	viewer.Stderr = os.Stderr

	if viewerErr := viewer.Run(); viewerErr != nil {
		return fmt.Errorf("run virt-viewer: %w", viewerErr)
	}

	return nil
}

func domainNodeConfig(name string) virt.NodeConfig {
	return virt.NodeConfig{
		Name:     name,
		CPUs:     1,
		MemoryMB: 1024,
	}
}
