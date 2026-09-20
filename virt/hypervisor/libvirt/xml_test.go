package libvirt

import (
	"errors"
	"testing"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/domain"
	"github.com/tethux/tethux/virt/hypervisor/libvirt/errs"
)

func TestBuildDomainCarriesDomainConfiguration(t *testing.T) {
	config := &domain.RuntimeConfig{
		NodeConfig:   virt.NodeConfig{Name: "router", CPUs: 2, MemoryMB: 512},
		Architecture: "x86_64", Machine: "q35", Firmware: domain.FirmwareUEFI,
		BootOrder: []domain.BootDevice{domain.BootDisk, domain.BootNetwork},
		Disks: []domain.RuntimeDisk{{
			Source: "/var/lib/tethux/router.qcow2", Bus: string(domain.DiskBusSATA),
			Target: "sda", Format: string(domain.DiskFormatQCOW2),
		}, {
			Source: "/var/lib/tethux/seed.iso", Device: string(domain.DiskDeviceCDROM),
			Bus: string(domain.DiskBusSATA), Target: "sdb", Format: string(domain.DiskFormatRaw), ReadOnly: true,
		}},
		Interfaces: []domain.Interface{{
			Bridge: "lab0", MAC: "52:54:00:12:34:56", Model: domain.InterfaceModelVirtio,
		}},
	}

	description, err := buildDomain(config)
	if err != nil {
		t.Fatal(err)
	}
	if description.Features == nil || description.Features.ACPI == nil || description.Features.APIC == nil {
		t.Fatal("domain is missing ACPI/APIC support")
	}
	if description.OS.Firmware != "efi" {
		t.Fatalf("firmware = %q, want efi", description.OS.Firmware)
	}
	if len(description.OS.BootDevices) != 2 || description.OS.BootDevices[0].Dev != "hd" || description.OS.BootDevices[1].Dev != "network" {
		t.Fatalf("boot devices = %#v", description.OS.BootDevices)
	}
	if description.Memory == nil || description.Memory.Value != 512 || description.VCPU == nil || description.VCPU.Value != 2 {
		t.Fatal("domain resources were not carried into XML")
	}
	if len(description.Devices.Disks) != 2 || description.Devices.Disks[0].Target.Bus != "sata" {
		t.Fatal("domain disk was not carried into XML")
	}
	if description.Devices.Disks[1].Device != "cdrom" || description.Devices.Disks[1].ReadOnly == nil {
		t.Fatal("domain optical media was not carried into XML")
	}
	if len(description.Devices.Interfaces) != 1 || description.Devices.Interfaces[0].Source.Bridge.Bridge != "lab0" {
		t.Fatal("domain interface was not carried into XML")
	}
}

func TestBuildDomainRejectsUnsupportedFirmware(t *testing.T) {
	_, err := buildDomain(&domain.RuntimeConfig{
		NodeConfig: virt.NodeConfig{Name: "bad-firmware"},
		Firmware:   domain.Firmware("coreboot"),
	})
	if !errors.Is(err, errs.ErrFirmware) {
		t.Fatalf("error = %v, want ErrFirmware", err)
	}
}
