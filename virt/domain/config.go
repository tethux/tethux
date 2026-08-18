package domain

import (
	"github.com/tethux/tethux/storage"
	"github.com/tethux/tethux/virt"
)

// Config describes a provider-independent virtual-machine domain.
type Config struct {
	virt.NodeConfig

	Machine      string
	Architecture string
	Firmware     Firmware

	Disks      []Disk
	Interfaces []Interface

	BootOrder []BootDevice
}

// Firmware identifies a domain firmware interface.
type Firmware string

const (
	// FirmwareBIOS selects legacy BIOS firmware.
	FirmwareBIOS Firmware = "bios"

	// FirmwareUEFI selects UEFI firmware.
	FirmwareUEFI Firmware = "uefi"
)

// DiskBus identifies the bus used to attach a disk.
type DiskBus string

const (
	DiskBusVirtio DiskBus = "virtio"
	DiskBusSATA   DiskBus = "sata"
	DiskBusSCSI   DiskBus = "scsi"
	DiskBusIDE    DiskBus = "ide"
	DiskBusUSB    DiskBus = "usb"
)

// DiskFormat identifies the storage format of a disk.
type DiskFormat string

const (
	DiskFormatRaw   DiskFormat = "raw"
	DiskFormatQCOW2 DiskFormat = "qcow2"
)

// Disk describes a block device attached to a domain.
type Disk struct {
	Source storage.Ref

	Bus    DiskBus
	Target string
	Format DiskFormat

	ReadOnly bool
}

// InterfaceModel identifies the emulated network device.
type InterfaceModel string

const (
	InterfaceModelVirtio InterfaceModel = "virtio"
	InterfaceModelE1000  InterfaceModel = "e1000"
)

// Interface describes a network interface attached to a domain.
type Interface struct {
	MAC    string
	Model  InterfaceModel
	Bridge string
}

// BootDevice identifies a domain boot source.
type BootDevice string

const (
	// BootDisk boots from a disk.
	BootDisk BootDevice = "disk"

	// BootCDROM boots from optical media.
	BootCDROM BootDevice = "cdrom"

	// BootNetwork boots from the network.
	BootNetwork BootDevice = "network"
)
