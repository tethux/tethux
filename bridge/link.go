package bridge

import (
	"fmt"
	"runtime"

	"github.com/tethux/tethux/bridge/errs"
	"github.com/tethux/tethux/bridge/models"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// NamespaceInterfaceOptions configures attachment of an interface to a network namespace.
type NamespaceInterfaceOptions struct {
	Mode              models.NamespaceInterfaceMode
	PID               int
	HostSideName      string
	ContainerSideName string
	MTU               int
}

// AttachNamespaceInterface creates or prepares an interface according to opts.
func AttachNamespaceInterface(opts NamespaceInterfaceOptions) error {
	switch opts.Mode {
	case "", models.NamespaceInterfaceCreateVeth:
		return AttachVethToNamespace(opts.PID, opts.HostSideName, opts.ContainerSideName, opts.MTU)
	case models.NamespaceInterfaceExisting:
		return PrepareExistingInterface(opts.HostSideName, opts.MTU)
	default:
		return errs.New("attach namespace interface", errs.ErrUnsupportedMode, string(opts.Mode))
	}
}

// AttachVethToNamespace creates a veth pair and moves its peer into a process namespace.
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

	if renameErr := netlink.LinkSetName(link, containerSideName); renameErr != nil {
		return errs.Wrap("rename namespace interface", errs.ErrFailedToCreate, peerName+" -> "+containerSideName, renameErr)
	}

	link, err = netlink.LinkByName(containerSideName)
	if err != nil {
		return errs.ErrLinkNotFound
	}

	setMtuErr := netlink.LinkSetMTU(link, mtu)
	if setMtuErr != nil {
		return errs.Wrap("set interface MTU", errs.ErrFailedToSetMTU, containerSideName, setMtuErr)
	}

	return netlink.LinkSetUp(link)
}

// PrepareExistingInterface configures and enables an existing host interface.
func PrepareExistingInterface(hostSideName string, mtu int) error {
	link, err := netlink.LinkByName(hostSideName)
	if err != nil {
		return errs.Wrap("find existing interface", errs.ErrLinkNotFound, hostSideName, err)
	}

	if mtu > 0 {
		if err := netlink.LinkSetMTU(link, mtu); err != nil {
			return errs.Wrap("set interface MTU", errs.ErrFailedToSetMTU, hostSideName, err)
		}
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return errs.Wrap("bring interface up", errs.ErrFailedToSetMTU, hostSideName, err)
	}

	return nil
}

func peerInterfaceName(hostSideName string) string {
	name := "p" + hostSideName
	if len(name) <= 15 {
		return name
	}
	return name[:15]
}

// SetupLinkWithNames creates a configured veth pair and moves its peer into a namespace.
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
		return errs.Wrap("create veth pair", errs.ErrFailedToCreate, params.HostName+" <-> "+params.Container, err)
	}

	peerLink, err := netlink.LinkByName(params.Container)
	if err != nil {
		return errs.Wrap("find peer interface", errs.ErrFailedToFindPeer, params.Container, err)
	}

	setMtuErr := netlink.LinkSetMTU(peerLink, params.MTU)
	if setMtuErr != nil {
		return errs.Wrap("set interface MTU", errs.ErrFailedToSetMTU, params.Container, setMtuErr)
	}

	targetNs, err := netns.GetFromPid(params.SourcePID)
	if err != nil {
		return errs.Wrap("get process namespace", errs.ErrNamespaceFailed, fmt.Sprint(params.SourcePID), err)
	}
	defer targetNs.Close()

	if setLintErr := netlink.LinkSetNsFd(peerLink, int(targetNs)); setLintErr != nil {
		return errs.Wrap("move interface to namespace", errs.ErrNamespaceFailed, params.Container, setLintErr)
	}

	hostLink, err := netlink.LinkByName(params.HostName)
	if err == nil {
		setLinkUpErr := netlink.LinkSetUp(hostLink)
		if setLinkUpErr != nil {
			return errs.Wrap("bring interface up", errs.ErrFailedToSetMTU, params.HostName, setLinkUpErr)
		}
	}

	return nil
}

// CleanupLink removes a host link when it exists.
func CleanupLink(hostName string) {
	link, err := netlink.LinkByName(hostName)
	if err == nil {
		delErr := netlink.LinkDel(link)
		if delErr != nil {
			fmt.Printf("failed to delete link %s: %v\n", hostName, delErr)
		}
	}
}

// EnterNamespace enters a process network namespace and returns a restoration function.
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
