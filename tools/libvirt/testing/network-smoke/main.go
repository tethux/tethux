// network-smoke boots three Alpine domains and verifies a bridged ICMP path.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/vishvananda/netlink"

	"github.com/tethux/tethux/storage"
	localstorage "github.com/tethux/tethux/storage/local"
	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/domain"
	libvirtprovider "github.com/tethux/tethux/virt/hypervisor/libvirt"
)

const successMarker = "TETHUX_LIBVIRT_NETWORK_OK"

type guest struct {
	name      string
	addresses []string
	macs      []string
	bridges   []string
	userData  string
	node      *domain.Node
	resources *domain.PreparedResources
}

func main() {
	os.Exit(mainExit())
}

func mainExit() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "libvirt network smoke: %v\n", err)
		return 1
	}
	return 0
}

func run(parent context.Context) error {
	var uri, image string
	flag.StringVar(&uri, "uri", "qemu:///system", "libvirt connection URI")
	flag.StringVar(&image, "image", os.Getenv("TETHUX_LIBVIRT_IMAGE"), "Alpine cloud qcow2 image")
	flag.Parse()
	if os.Geteuid() != 0 {
		return errors.New("root is required to create isolated host bridges")
	}
	if image == "" {
		return errors.New("set --image or TETHUX_LIBVIRT_IMAGE to an Alpine cloud qcow2 image")
	}
	image, err := filepath.Abs(image)
	if err != nil {
		return fmt.Errorf("resolve image: %w", err)
	}
	if _, statErr := os.Stat(image); statErr != nil {
		return fmt.Errorf("inspect image: %w", statErr)
	}

	ctx, cancel := context.WithTimeout(parent, 4*time.Minute)
	defer cancel()
	workDir, err := os.MkdirTemp("/var/tmp", "tethux-libvirt-")
	if err != nil {
		return fmt.Errorf("create work directory: %w", err)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(workDir); cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "remove work directory: %v\n", cleanupErr)
		}
	}()
	if chmodErr := os.Chmod(workDir, 0o755); chmodErr != nil { // #nosec G302 -- the unprivileged qemu service must traverse this temporary directory.
		return fmt.Errorf("make work directory accessible to qemu: %w", chmodErr)
	}

	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	bridgeA := "tx" + suffix + "a"
	bridgeB := "tx" + suffix + "b"
	if bridgeErr := createBridge(bridgeA); bridgeErr != nil {
		return bridgeErr
	}
	defer deleteLink(bridgeA)
	if bridgeErr := createBridge(bridgeB); bridgeErr != nil {
		return bridgeErr
	}
	defer deleteLink(bridgeB)

	base := filepath.Join(workDir, "alpine.qcow2")
	if copyErr := copyFile(image, base); copyErr != nil {
		return copyErr
	}
	storageProvider, err := localstorage.New(workDir)
	if err != nil {
		return err
	}
	manager := domain.NewManager(storageProvider)
	provider, err := libvirtprovider.New(uri)
	if err != nil {
		return err
	}
	defer provider.Close()
	events, err := provider.Events(ctx)
	if err != nil {
		return err
	}

	guests := []*guest{
		{
			name: "tethux-alpine-b-" + suffix, addresses: []string{"10.86.0.2/24"},
			macs: []string{"52:54:00:86:00:02"}, bridges: []string{bridgeB},
		},
		{
			name: "tethux-alpine-switch-" + suffix,
			macs: []string{"52:54:00:86:10:01", "52:54:00:86:10:02"}, bridges: []string{bridgeA, bridgeB},
			userData: switchUserData(),
		},
		{
			name: "tethux-alpine-a-" + suffix, addresses: []string{"10.86.0.1/24"},
			macs: []string{"52:54:00:86:00:01"}, bridges: []string{bridgeA},
			userData: endpointUserData(),
		},
	}
	defer cleanupGuests(manager, provider, guests)

	for _, value := range guests {
		if prepareErr := prepareGuest(ctx, workDir, base, value); prepareErr != nil {
			return prepareErr
		}
		config := guestConfig(storageProvider.Name(), value)
		value.node, value.resources, err = manager.Create(ctx, provider, config)
		if err != nil {
			return fmt.Errorf("create %s: %w", value.name, err)
		}
		fmt.Printf("started %s (%s)\n", value.node.Name, value.node.ID)
	}

	if err := verifyProviderSurface(ctx, provider, guests); err != nil {
		return err
	}
	consoleCtx, stopConsole := context.WithCancel(ctx)
	marker := &markerOutput{output: os.Stdout, marker: successMarker, cancel: stopConsole}
	consoleErr := provider.OpenConsole(consoleCtx, guests[2].node.ID, nil, marker)
	stopConsole()
	if !marker.found {
		return fmt.Errorf("endpoint console closed before %s: %w", successMarker, consoleErr)
	}
	if consoleErr != nil && !errors.Is(consoleErr, context.Canceled) {
		return consoleErr
	}

	if err := exerciseLifecycle(ctx, provider, guests); err != nil {
		return err
	}
	started := 0
	drain := time.NewTimer(500 * time.Millisecond)
	defer drain.Stop()
eventLoop:
	for {
		select {
		case event := <-events:
			if event.Type == virt.EventStarted {
				started++
			}
		case <-drain.C:
			break eventLoop
		}
	}
	if started < len(guests) {
		return fmt.Errorf("received %d started events, want at least %d", started, len(guests))
	}

	fmt.Printf("PASS: three Alpine domains carried ICMP through the Alpine switch (%d lifecycle starts observed)\n", started)
	return nil
}

func guestConfig(provider storage.ProviderName, value *guest) *domain.Config {
	config := &domain.Config{
		NodeConfig:   virt.NodeConfig{Name: value.name, CPUs: 1, MemoryMB: 256},
		Architecture: "x86_64", Machine: "q35", BootOrder: []domain.BootDevice{domain.BootDisk},
		Disks: []domain.Disk{
			{Source: storage.Ref{Provider: provider, Key: storage.Key(value.name + ".qcow2")}, Bus: domain.DiskBusSATA, Target: "sda", Format: domain.DiskFormatQCOW2},
			{
				Source: storage.Ref{Provider: provider, Key: storage.Key(value.name + "-seed.iso")},
				Device: domain.DiskDeviceCDROM, Bus: domain.DiskBusSATA, Target: "sdb",
				Format: domain.DiskFormatRaw, ReadOnly: true,
			},
		},
	}
	for index, bridge := range value.bridges {
		config.Interfaces = append(config.Interfaces, domain.Interface{
			Bridge: bridge, MAC: value.macs[index], Model: domain.InterfaceModelVirtio,
		})
	}
	return config
}

func prepareGuest(ctx context.Context, workDir, base string, value *guest) error {
	disk := filepath.Join(workDir, value.name+".qcow2")
	if err := command(ctx, "qemu-img", "create", "-q", "-f", "qcow2", "-F", "qcow2", "-b", base, disk); err != nil {
		return err
	}
	if err := os.Chmod(disk, 0o644); err != nil { // #nosec G302 -- the unprivileged qemu service must read and write this overlay.
		return fmt.Errorf("make overlay accessible to qemu: %w", err)
	}
	seedDir := filepath.Join(workDir, value.name+"-seed")
	if err := os.Mkdir(seedDir, 0o755); err != nil { // #nosec G301 -- xorriso reads this short-lived directory as the invoking root user.
		return fmt.Errorf("create seed directory: %w", err)
	}
	userData := value.userData
	if userData == "" {
		userData = "#!/bin/sh\n"
	}
	files := map[string]string{
		"user-data": userData,
		"meta-data": metadata(value),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(seedDir, name), []byte(content), 0o644); err != nil { // #nosec G306 -- seed data is intentionally non-secret and embedded in a world-readable image.
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	seed := filepath.Join(workDir, value.name+"-seed.iso")
	if err := command(ctx, "xorrisofs", "-quiet", "-output", seed, "-volid", "cidata", "-joliet", "-rock",
		filepath.Join(seedDir, "user-data"), filepath.Join(seedDir, "meta-data")); err != nil {
		return err
	}
	if err := os.Chmod(seed, 0o644); err != nil { // #nosec G302 -- the unprivileged qemu service must read this seed image.
		return fmt.Errorf("make seed accessible to qemu: %w", err)
	}
	return nil
}

func metadata(value *guest) string {
	var output strings.Builder
	fmt.Fprintf(&output, "instance-id: %s\nhostname: %s\nnetwork-interfaces: |\n", value.name, value.name)
	output.WriteString("  auto lo\n  iface lo inet loopback\n")
	for index := range value.macs {
		fmt.Fprintf(&output, "  auto eth%d\n  iface eth%d inet ", index, index)
		if index < len(value.addresses) && value.addresses[index] != "" {
			address, _, _ := strings.Cut(value.addresses[index], "/")
			fmt.Fprintf(&output, "static\n    address %s\n    netmask 255.255.255.0\n", address)
		} else {
			output.WriteString("manual\n")
		}
	}
	return output.String()
}

func switchUserData() string {
	return `#!/bin/sh
set -eu
modprobe bridge
brctl addbr br0
brctl setfd br0 0
brctl stp br0 off
brctl addif br0 eth0
brctl addif br0 eth1
ip link set eth0 up
ip link set eth1 up
ip link set br0 up
`
}

func endpointUserData() string {
	return `#!/bin/sh
until ping -c 3 -W 1 10.86.0.2; do sleep 1; done
echo TETHUX_LIBVIRT_NETWORK_OK >/dev/ttyS0
`
}

func verifyProviderSurface(ctx context.Context, provider *libvirtprovider.Provider, guests []*guest) error {
	nodes, err := provider.List(ctx)
	if err != nil {
		return err
	}
	if len(nodes) < len(guests) {
		return fmt.Errorf("list returned %d managed domains, want at least %d", len(nodes), len(guests))
	}
	for _, value := range guests {
		state, err := provider.State(ctx, value.node.ID)
		if err != nil || state != virt.NodeRunning {
			return fmt.Errorf("state %s: got %s: %w", value.name, state, err)
		}
		inspected, err := provider.InspectDomain(ctx, value.node.ID)
		if err != nil {
			return err
		}
		if len(inspected.Disks) != 2 || len(inspected.Interfaces) != len(value.bridges) || inspected.Aux == nil {
			return fmt.Errorf("inspect %s returned incomplete devices", value.name)
		}
		if _, err := provider.Reload(ctx, value.node.ID); err != nil {
			return err
		}
	}
	return nil
}

func exerciseLifecycle(ctx context.Context, provider *libvirtprovider.Provider, guests []*guest) error {
	switchID := guests[1].node.ID
	if err := provider.Suspend(ctx, switchID); err != nil {
		return err
	}
	if state, err := provider.State(ctx, switchID); err != nil || state != virt.NodeSuspended {
		return fmt.Errorf("suspended state: got %s: %w", state, err)
	}
	if err := provider.Resume(ctx, switchID); err != nil {
		return err
	}
	if err := provider.Restart(ctx, switchID); err != nil {
		return err
	}

	endpointID := guests[2].node.ID
	stopCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := provider.Stop(stopCtx, endpointID); err != nil {
		return err
	}
	if err := provider.Start(ctx, endpointID); err != nil {
		return err
	}
	if err := provider.PowerOff(ctx, endpointID); err != nil {
		return err
	}
	return nil
}

type markerOutput struct {
	output io.Writer
	marker string
	cancel context.CancelFunc
	buffer bytes.Buffer
	found  bool
}

func (w *markerOutput) Write(data []byte) (int, error) {
	count, err := w.output.Write(data)
	if w.found {
		return count, err
	}
	_, _ = w.buffer.Write(data)
	if strings.Contains(w.buffer.String(), w.marker) {
		w.found = true
		w.cancel()
	} else if w.buffer.Len() > len(w.marker)*2 {
		value := w.buffer.Bytes()
		w.buffer.Reset()
		_, _ = w.buffer.Write(value[len(value)-len(w.marker):])
	}
	return count, err
}

func cleanupGuests(manager *domain.Manager, provider *libvirtprovider.Provider, guests []*guest) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for index := len(guests) - 1; index >= 0; index-- {
		value := guests[index]
		if value.node != nil {
			if err := provider.Delete(ctx, value.node.ID); err != nil {
				fmt.Fprintf(os.Stderr, "cleanup %s: %v\n", value.name, err)
			}
		}
		if err := manager.Release(ctx, value.resources); err != nil {
			fmt.Fprintf(os.Stderr, "release %s storage: %v\n", value.name, err)
		}
	}
}

func createBridge(name string) error {
	link := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: name}}
	if err := netlink.LinkAdd(link); err != nil {
		return fmt.Errorf("create bridge %s: %w", name, err)
	}
	if err := netlink.LinkSetUp(link); err != nil {
		deleteLink(name)
		return fmt.Errorf("bring bridge %s up: %w", name, err)
	}
	return nil
}

func deleteLink(name string) {
	if link, err := netlink.LinkByName(name); err == nil {
		_ = netlink.LinkDel(link)
	}
}

func copyFile(source, destination string) error {
	// #nosec G304 -- both paths are rooted in an operator-selected image and a validated MkdirTemp directory.
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open Alpine image: %w", err)
	}
	defer input.Close()
	// #nosec G304,G302 -- destination is under the validated temporary directory and must be readable by qemu.
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create local Alpine image: %w", err)
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return fmt.Errorf("copy Alpine image: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close local Alpine image: %w", err)
	}
	return nil
}

func command(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- callers select fixed qemu-img and xorrisofs commands; only validated paths vary.
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run %s: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}
