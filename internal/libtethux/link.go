package libtethux

import (
	"fmt"
	"runtime"

	"github.com/0xveya/tethux/internal/libtethux/errs"
	"github.com/0xveya/tethux/internal/libtethux/models"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

func AttachVethToNamespace(pid int, hostSideName, containerSideName string, mtu int) error {
	peerName := peerInterfaceName(hostSideName)
	err := SetupLinkWithNames(models.SetupLinkParams{
		SourcePID: pid,
		HostName:  hostSideName,
		Container: peerName,
		MTU:       mtu,
	})
	if err != nil {
		return err
	}

	cleanup, err := EnterNamespace(pid)
	if err != nil {
		return err
	}
	defer cleanup()

	link, err := netlink.LinkByName(peerName)
	if err != nil {
		return errs.ErrLinkNotFound
	}

	if err := netlink.LinkSetName(link, containerSideName); err != nil {
		return fmt.Errorf("%w: failed to rename %s to %s: %w", errs.ErrFailedToCreate, peerName, containerSideName, err)
	}

	link, err = netlink.LinkByName(containerSideName)
	if err != nil {
		return errs.ErrLinkNotFound
	}

	setMtuErr := netlink.LinkSetMTU(link, mtu)
	if setMtuErr != nil {
		return fmt.Errorf("%w: failed to set MTU for %s: %w", errs.ErrFailedToSetMTU, containerSideName, setMtuErr)
	}

	return netlink.LinkSetUp(link)
}

func peerInterfaceName(hostSideName string) string {
	name := "p" + hostSideName
	if len(name) <= 15 {
		return name
	}
	return name[:15]
}

func SetupLinkWithNames(params models.SetupLinkParams) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: params.HostName,
			MTU:  params.MTU,
		},
		PeerName: params.Container,
	}

	if err := netlink.LinkAdd(veth); err != nil {
		return fmt.Errorf("%w: failed to create veth pair (%s <-> %s): %w", errs.ErrFailedToCreate, params.HostName, params.Container, err)
	}

	peerLink, err := netlink.LinkByName(params.Container)
	if err != nil {
		return fmt.Errorf("%w: failed to find peer interface: %w", errs.ErrFailedToFindPeer, err)
	}

	setMtuErr := netlink.LinkSetMTU(peerLink, params.MTU)
	if setMtuErr != nil {
		return fmt.Errorf("%w: failed to set MTU for %s: %w", errs.ErrFailedToSetMTU, params.Container, setMtuErr)
	}

	targetNs, err := netns.GetFromPid(params.SourcePID)
	if err != nil {
		return fmt.Errorf("%w: failed to get namespace for PID %d: %w", errs.ErrNamespaceFailed, params.SourcePID, err)
	}
	defer targetNs.Close()

	if setLintErr := netlink.LinkSetNsFd(peerLink, int(targetNs)); setLintErr != nil {
		return fmt.Errorf("%w: failed to move %s to PID %d: %w", errs.ErrNamespaceFailed, params.Container, params.SourcePID, setLintErr)
	}

	hostLink, err := netlink.LinkByName(params.HostName)
	if err == nil {
		setLinkUpErr := netlink.LinkSetUp(hostLink)
		if setLinkUpErr != nil {
			return fmt.Errorf("%w: failed to set up %s: %w", errs.ErrFailedToSetMTU, params.HostName, setLinkUpErr)
		}
	}

	return nil
}

func CleanupLink(hostName string) {
	link, err := netlink.LinkByName(hostName)
	if err == nil {
		delErr := netlink.LinkDel(link)
		if delErr != nil {
			fmt.Printf("failed to delete link %s: %v\n", hostName, delErr)
		}
	}
}

func EnterNamespace(pid int) (func(), error) {
	runtime.LockOSThread()

	hostNS, err := netns.Get()
	if err != nil {
		runtime.UnlockOSThread()
		return nil, err
	}

	targetNS, err := netns.GetFromPid(pid)
	if err != nil {
		closedErr := hostNS.Close()
		if closedErr != nil {
			fmt.Printf("failed to close host namespace: %v\n", closedErr)
		}
		runtime.UnlockOSThread()
		return nil, err
	}

	if err := netns.Set(targetNS); err != nil {
		closeTargetNSErr := targetNS.Close()
		if closeTargetNSErr != nil {
			fmt.Printf("failed to close target namespace: %v\n", closeTargetNSErr)
		}
		closedErr := hostNS.Close()
		if closedErr != nil {
			fmt.Printf("failed to close host namespace: %v\n", closedErr)
		}
		runtime.UnlockOSThread()
		return nil, err
	}

	return func() {
		defer runtime.UnlockOSThread()
		defer hostNS.Close()
		defer targetNS.Close()

		setNSErr := netns.Set(hostNS)
		if setNSErr != nil {
			fmt.Printf("failed to switch back to host namespace: %v\n", setNSErr)
		}
	}, nil
}
