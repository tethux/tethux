package virt

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tethux/tethux/storage"
	localstorage "github.com/tethux/tethux/storage/local"
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
		uri        string
		name       string
		id         string
		disk       string
		action     string
		keep       bool
		bridges    []string
		macs       []string
		format     string
		diskBus    string
		diskTarget string
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
				return createLibvirtDomain(ctx, provider, name, disk, format, diskBus, diskTarget, bridges, macs, keep)

			case "list":
				return listLibvirtDomains(ctx, provider)

			case "start",
				"stop",
				"poweroff",
				"suspend",
				"resume",
				"restart",
				"delete",
				"inspect",
				"reload",
				"state",
				"console",
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
	flags.StringVar(&format, "format", string(domain.DiskFormatQCOW2), "disk format: qcow2 or raw")
	flags.StringVar(&diskBus, "disk-bus", string(domain.DiskBusVirtio), "disk bus: virtio, sata, scsi, ide, or usb")
	flags.StringVar(&diskTarget, "disk-target", "vda", "guest disk target, such as vda or sda")
	flags.StringSliceVar(&bridges, "bridge", nil, "host bridge to attach; repeat for multiple interfaces")
	flags.StringSliceVar(&macs, "mac", nil, "MAC address matching each --bridge")

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
		"action: create, list, inspect, state, reload, start, stop, poweroff, suspend, resume, restart, delete, console, view",
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
	format string,
	diskBus string,
	diskTarget string,
	bridges []string,
	macs []string,
	keep bool,
) error {
	if disk == "" {
		return fmt.Errorf("--disk is required when --action=create")
	}

	absoluteDisk, pathErr := filepath.Abs(disk)
	if pathErr != nil {
		return fmt.Errorf("resolve disk path: %w", pathErr)
	}

	if len(macs) > len(bridges) {
		return fmt.Errorf("--mac cannot be specified without a matching --bridge")
	}
	storageProvider, storageErr := localstorage.New(filepath.Dir(absoluteDisk))
	if storageErr != nil {
		return storageErr
	}
	manager := domain.NewManager(storageProvider)
	config := &domain.Config{
		NodeConfig: domainNodeConfig(name),

		Architecture: defaultArchitecture,
		Machine:      defaultMachine,

		Disks: []domain.Disk{
			{
				Source: storage.Ref{Provider: storageProvider.Name(), Key: storage.Key(filepath.Base(absoluteDisk))},
				Bus:    domain.DiskBus(diskBus),
				Target: diskTarget,
				Format: domain.DiskFormat(format),
			},
		},
	}
	for index, bridge := range bridges {
		iface := domain.Interface{Bridge: bridge, Model: domain.InterfaceModelVirtio}
		if index < len(macs) {
			iface.MAC = macs[index]
		}
		config.Interfaces = append(config.Interfaces, iface)
	}

	node, resources, createErr := manager.Create(ctx, provider, config)
	if createErr != nil {
		return createErr
	}
	defer func() { _ = manager.Release(context.Background(), resources) }()

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

	case "inspect":
		node, err := provider.InspectDomain(ctx, id)
		if err != nil {
			return err
		}
		printLibvirtDomain(node)
		return nil

	case "reload":
		node, err := provider.Reload(ctx, id)
		if err != nil {
			return err
		}
		fmt.Printf("%s %s %s\n", node.ID, node.Name, node.State)
		return nil

	case "state":
		state, err := provider.State(ctx, id)
		if err != nil {
			return err
		}
		fmt.Println(state)
		return nil

	case "console":
		return provider.OpenConsole(ctx, id, os.Stdin, os.Stdout)

	case "view":
		return viewLibvirtDomain(ctx, provider, uri, id)

	default:
		return fmt.Errorf("unsupported domain action %q", action)
	}
}

func printLibvirtDomain(node *domain.Node) {
	fmt.Printf("%s\n  id:    %s\n  state: %s\n", node.Name, node.ID, node.State)
	for _, disk := range node.Disks {
		fmt.Printf("  disk:  %s %s\n", disk.Target, disk.Source)
	}
	for _, iface := range node.Interfaces {
		fmt.Printf("  nic:   %s %s\n", iface.MAC, iface.Target)
	}
	if node.Aux != nil {
		fmt.Printf("  spice: %s:%d\n", node.Aux.Host, node.Aux.Port)
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
