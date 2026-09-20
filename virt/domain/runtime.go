package domain

import "github.com/tethux/tethux/virt"

// RuntimeConfig contains domain configuration with storage references resolved.
type RuntimeConfig struct {
	virt.NodeConfig

	Machine      string
	Architecture string
	Firmware     Firmware

	Disks      []RuntimeDisk
	Interfaces []Interface

	BootOrder []BootDevice
}

// RuntimeDisk describes a disk path ready for a hypervisor.
type RuntimeDisk struct {
	Source string

	Device string
	Bus    string
	Target string
	Format string

	ReadOnly bool
}
