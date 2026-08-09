package domain

import "github.com/tethux/tethux/internal/libtethux/virt"

type Node struct {
	virt.Node

	UUID       string
	Persistent bool

	Disks      []DiskInfo
	Interfaces []InterfaceInfo
}

type DiskInfo struct {
	Target string
	Source string
}

type InterfaceInfo struct {
	MAC    string
	Target string
}
