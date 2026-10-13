package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/tethux/tethux/bridge"
	"github.com/tethux/tethux/storage"
	localstorage "github.com/tethux/tethux/storage/local"
	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/domain"
	"github.com/vishvananda/netlink"
)

// DomainOptions selects a caller-owned hypervisor and optional image override.
type DomainOptions struct {
	Provider  domain.Provider
	Image     string
	SeedImage string
}

type domainResources struct {
	provider  domain.Provider
	manager   *domain.Manager
	prepared  *domain.PreparedResources
	id        string
	directory string
	links     []netlink.Link
	switches  []*bridge.Switch
}

func (run *Running) startDomain(ctx context.Context, top *topology.Topology, node *topology.Node, spec *topology.DomainSpec, options Options, token string, index int) error {
	directory, directoryErr := os.MkdirTemp("/var/tmp", "tethux-domain-")
	if directoryErr != nil {
		return directoryErr
	}
	resources := &domainResources{provider: options.Domains.Provider, directory: directory}
	run.domains[node.ID] = resources
	run.nodes[node.ID] = ""
	run.order = append(run.order, node.ID)
	if chmodErr := os.Chmod(directory, 0o755); chmodErr != nil {
		return chmodErr
	} // #nosec G302 -- The unprivileged QEMU service must traverse its isolated work directory.
	store, storeErr := localstorage.New(directory)
	if storeErr != nil {
		return storeErr
	}
	resources.manager = domain.NewManager(store)
	image := spec.Image
	if options.Domains.Image != "" {
		image = options.Domains.Image
	}
	seedImage := spec.SeedImage
	if options.Domains.SeedImage != "" {
		seedImage = options.Domains.SeedImage
	}
	absolute, absoluteErr := filepath.Abs(image)
	if absoluteErr != nil {
		return absoluteErr
	}
	overlay := filepath.Join(directory, "disk.qcow2")
	create := exec.CommandContext(ctx, "qemu-img", "create", "-q", "-f", "qcow2", "-F", "qcow2", "-b", absolute, overlay) // #nosec G204 -- Operator-selected image and owned overlay paths are structured arguments.
	if output, createErr := create.CombinedOutput(); createErr != nil {
		return errs.Wrap("create domain overlay", errs.ErrStart, string(output), createErr)
	}
	if chmodErr := os.Chmod(overlay, 0o644); chmodErr != nil {
		return chmodErr
	} // #nosec G302 -- libvirt assigns overlay ownership to its QEMU service.
	cfg := &domain.Config{
		NodeConfig: virt.NodeConfig{ID: uuid.NewString(), Name: fmt.Sprintf("tethux-%s-%d", token, index), CPUs: int(spec.CPUs), MemoryMB: int(spec.MemoryMB)},
		BootOrder:  []domain.BootDevice{domain.BootDisk}, Disks: []domain.Disk{{Source: storage.Ref{Provider: store.Name(), Key: "disk.qcow2"}, Bus: domain.DiskBusSATA, Target: "sda", Format: domain.DiskFormatQCOW2}},
	}
	if seedImage != "" {
		if copyErr := copyDomainSeed(seedImage, filepath.Join(directory, "seed.iso")); copyErr != nil {
			return copyErr
		}
		cfg.Disks = append(cfg.Disks, domain.Disk{Source: storage.Ref{Provider: store.Name(), Key: "seed.iso"}, Device: domain.DiskDeviceCDROM, Bus: domain.DiskBusSATA, Target: "sdb", Format: domain.DiskFormatRaw, ReadOnly: true})
	}
	for portIndex, port := range node.Ports {
		if run.nextDomainPort > 65535 {
			return errs.New("create domain interface", errs.ErrUnsupported, "too many domain ports")
		}
		name := fmt.Sprintf("tx%sg%03x", token, run.nextDomainPort)
		run.nextDomainPort++
		link := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: name}}
		if linkErr := netlink.LinkAdd(link); linkErr != nil {
			return linkErr
		}
		resources.links = append(resources.links, link)
		if linkErr := netlink.LinkSetUp(link); linkErr != nil {
			return linkErr
		}
		// External endpoint MACs must reach the bridge's packet socket.
		if promiscErr := netlink.SetPromiscOn(link); promiscErr != nil {
			return promiscErr
		}
		cfg.Interfaces = append(cfg.Interfaces, domain.Interface{Bridge: name, Model: domain.InterfaceModelVirtio, MAC: fmt.Sprintf("52:54:%02x:%02x:%02x:%02x", (index>>8)&255, index&255, (portIndex>>8)&255, portIndex&255)})
		for linkIndex, connection := range top.Links {
			for side, endpoint := range []topology.Endpoint{connection.A, connection.B} {
				if endpoint.Node == node.ID && endpoint.Port == port.ID {
					if switchErr := resources.connect(ctx, name, options.BasePort+2*linkIndex+side, options.BasePort+2*linkIndex+1-side, int(connection.MTU)); switchErr != nil {
						return switchErr
					}
				}
			}
		}
	}
	created, prepared, createErr := resources.manager.Create(ctx, resources.provider, cfg)
	resources.prepared = prepared
	if created != nil {
		resources.id = created.ID
		run.nodes[node.ID] = created.ID
	}
	if createErr != nil {
		return createErr
	}
	run.logger.InfoContext(ctx, "Domain running", "node", node.ID, "domain", created.ID)
	return nil
}

func copyDomainSeed(source, destination string) (resultErr error) {
	input, openErr := os.Open(source) // #nosec G304 -- Explicit operator-supplied seed image.
	if openErr != nil {
		return openErr
	}
	defer func() { resultErr = errors.Join(resultErr, input.Close()) }()
	output, createErr := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644) // #nosec G304,G302 -- Owned work-directory file, read by QEMU.
	if createErr != nil {
		return createErr
	}
	defer func() { resultErr = errors.Join(resultErr, output.Close()) }()
	_, copyErr := io.Copy(output, input)
	return copyErr
}

func (resources *domainResources) connect(ctx context.Context, name string, local, remote, mtu int) (resultErr error) {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if mtu == 0 {
		mtu = 1500
	}
	raw, rawErr := bridge.NewRawSocketPort(&bridge.PortOptions{ID: "domain", Interface: name, MTU: mtu})
	if rawErr != nil {
		return rawErr
	}
	udp, udpErr := bridge.NewUDPPort(&bridge.PortOptions{ID: "udp", LocalAddr: fmt.Sprintf("127.0.0.1:%d", local), Remote: fmt.Sprintf("127.0.0.1:%d", remote), MTU: mtu})
	if udpErr != nil {
		return errors.Join(udpErr, raw.Close())
	}
	switcher := bridge.NewSwitch(bridge.SwitchOptions{})
	if attachErr := switcher.AttachPort(raw); attachErr != nil {
		return errors.Join(attachErr, raw.Close(), udp.Close())
	}
	if attachErr := switcher.AttachPort(udp); attachErr != nil {
		return errors.Join(attachErr, raw.Close(), udp.Close())
	}
	if startErr := switcher.Start(); startErr != nil {
		return errors.Join(startErr, raw.Close(), udp.Close())
	}
	resources.switches = append(resources.switches, switcher)
	return nil
}

func (resources *domainResources) close(ctx context.Context) error {
	var failures []error
	remainingSwitches := resources.switches[:0]
	for _, switcher := range resources.switches {
		if stopErr := switcher.Stop(); stopErr != nil {
			failures = append(failures, stopErr)
			remainingSwitches = append(remainingSwitches, switcher)
		}
	}
	resources.switches = remainingSwitches
	if resources.id != "" {
		if deleteErr := resources.provider.Delete(ctx, resources.id); deleteErr != nil {
			return errors.Join(append(failures, deleteErr)...)
		}
		resources.id = ""
	}
	if resources.prepared != nil {
		if releaseErr := resources.manager.Release(ctx, resources.prepared); releaseErr != nil {
			return errors.Join(append(failures, releaseErr)...)
		}
		resources.prepared = nil
	}
	remaining := resources.links[:0]
	for _, link := range resources.links {
		if deleteErr := netlink.LinkDel(link); deleteErr != nil {
			failures = append(failures, deleteErr)
			remaining = append(remaining, link)
		}
	}
	resources.links = remaining
	if len(failures) != 0 {
		return errors.Join(failures...)
	}
	return os.RemoveAll(resources.directory)
}
