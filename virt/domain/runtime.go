package domain

import "github.com/tethux/tethux/virt"

type RuntimeConfig struct {
	virt.NodeConfig

	Machine      string
	Architecture string
	Firmware     Firmware

	Disks      []RuntimeDisk
	Interfaces []Interface

	BootOrder []BootDevice
}

type RuntimeDisk struct {
	Source string

	Bus    string
	Target string
	Format string

	ReadOnly bool
}
