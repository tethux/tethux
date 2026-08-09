package domain

import "github.com/tethux/tethux/internal/libtethux/virt"

type Config struct {
	virt.NodeConfig

	Machine      string
	Architecture string
	Firmware     Firmware

	Disks      []Disk
	Interfaces []Interface

	BootOrder []BootDevice
}

type Firmware string

const (
	FirmwareBIOS Firmware = "bios"
	FirmwareUEFI Firmware = "uefi"
)

type Disk struct {
	Source string

	Bus    string
	Target string
	Format string

	ReadOnly bool
}

type Interface struct {
	MAC    string
	Model  string
	Bridge string
}

type BootDevice string

const (
	BootDisk    BootDevice = "disk"
	BootCDROM   BootDevice = "cdrom"
	BootNetwork BootDevice = "network"
)
