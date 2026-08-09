package domain

import "github.com/tethux/tethux/virt"

// Node describes a virtual-machine domain and its runtime state.
type Node struct {
	virt.Node

	UUID       string
	Persistent bool

	Disks      []DiskInfo
	Interfaces []InterfaceInfo
}

// DiskInfo describes a disk attached to a running domain.
type DiskInfo struct {
	Target string
	Source string
}

// InterfaceInfo describes an interface attached to a running domain.
type InterfaceInfo struct {
	MAC    string
	Target string
}
