package libvirt

import (
	"errors"
	"fmt"
	"net"

	"github.com/google/uuid"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"

	"github.com/tethux/tethux/virt/domain"
	"github.com/tethux/tethux/virt/hypervisor/libvirt/errs"
)

const (
	metadataURI = "urn:tethux"
	metadataKey = "managed"

	defaultArchitecture = "x86_64"
	defaultMachine      = "q35"
)

const metadataXML = `
<tethux:tethux xmlns:tethux="urn:tethux">
	<tethux:managed/>
</tethux:tethux>`

func markManaged(dom *libvirt.Domain) error {
	metadataErr := dom.SetMetadata(
		libvirt.DOMAIN_METADATA_ELEMENT,
		metadataXML,
		metadataKey,
		metadataURI,
		libvirt.DOMAIN_AFFECT_CONFIG,
	)
	if metadataErr != nil {
		return errs.Wrap(errs.ErrMetadata, "", metadataErr)
	}

	return nil
}

func isManaged(dom *libvirt.Domain) (bool, error) {
	_, metadataErr := dom.GetMetadata(
		libvirt.DOMAIN_METADATA_ELEMENT,
		metadataURI,
		libvirt.DOMAIN_AFFECT_CONFIG,
	)
	if metadataErr == nil {
		return true, nil
	}

	var libvirtErr libvirt.Error
	if errors.As(metadataErr, &libvirtErr) &&
		libvirtErr.Code == libvirt.ERR_NO_DOMAIN_METADATA {
		return false, nil
	}

	return false, errs.Wrap(errs.ErrMetadata, "", metadataErr)
}

func domainXML(cfg *domain.RuntimeConfig) (string, error) {
	d, buildErr := buildDomain(cfg)
	if buildErr != nil {
		return "", buildErr
	}

	output, marshalErr := d.Marshal()
	if marshalErr != nil {
		return "", errs.Wrap(errs.ErrXML, d.Name, marshalErr)
	}

	return output, nil
}

func buildDomain(cfg *domain.RuntimeConfig) (*libvirtxml.Domain, error) {
	if cfg == nil {
		return nil, errs.New(
			errors.Join(errs.ErrConfig, errs.ErrNilConfig),
			"",
		)
	}

	name := cfg.Name
	if name == "" {
		name = cfg.ID
	}
	if name == "" {
		return nil, errs.New(
			errors.Join(errs.ErrConfig, errs.ErrEmptyName),
			"",
		)
	}

	if cfg.MemoryMB < 0 {
		return nil, errs.New(
			errors.Join(errs.ErrConfig, errs.ErrInvalidMemory),
			fmt.Sprintf("%d MiB", cfg.MemoryMB),
		)
	}

	if cfg.CPUs < 0 {
		return nil, errs.New(
			errors.Join(errs.ErrConfig, errs.ErrInvalidCPU),
			fmt.Sprintf("%d", cfg.CPUs),
		)
	}

	architecture := cfg.Architecture
	if architecture == "" {
		architecture = defaultArchitecture
	}

	machine := cfg.Machine
	if machine == "" {
		machine = defaultMachine
	}

	d := &libvirtxml.Domain{
		Type: "kvm",
		Name: name,

		OS: &libvirtxml.DomainOS{
			Type: &libvirtxml.DomainOSType{
				Type:    "hvm",
				Arch:    architecture,
				Machine: machine,
			},
		},

		Devices: &libvirtxml.DomainDeviceList{
			Serials: []libvirtxml.DomainSerial{
				{
					Source: &libvirtxml.DomainChardevSource{
						Pty: &libvirtxml.DomainChardevSourcePty{},
					},
				},
			},

			Consoles: []libvirtxml.DomainConsole{
				{
					Source: &libvirtxml.DomainChardevSource{
						Pty: &libvirtxml.DomainChardevSourcePty{},
					},
				},
			},

			Graphics: []libvirtxml.DomainGraphic{
				{
					Spice: &libvirtxml.DomainGraphicSpice{
						AutoPort: "yes",
						Listen:   "127.0.0.1",
					},
				},
			},
		},
	}

	if cfg.ID != "" {
		id, uuidErr := uuid.Parse(cfg.ID)
		if uuidErr != nil {
			return nil, errs.Wrap(
				errors.Join(errs.ErrConfig, errs.ErrInvalidUUID),
				cfg.ID,
				uuidErr,
			)
		}

		d.UUID = id.String()
	}

	if cfg.MemoryMB > 0 {
		d.Memory = &libvirtxml.DomainMemory{
			Unit:  "MiB",
			Value: uint(cfg.MemoryMB),
		}
	}

	if cfg.CPUs > 0 {
		d.VCPU = &libvirtxml.DomainVCPU{
			Value: uint(cfg.CPUs),
		}
	}

	for index, disk := range cfg.Disks {
		validationErr := validateDisk(disk)
		if validationErr != nil {
			return nil, errs.Wrap(
				errors.Join(errs.ErrConfig, errs.ErrDisk),
				fmt.Sprintf("disk %d", index),
				validationErr,
			)
		}

		d.Devices.Disks = append(
			d.Devices.Disks,
			buildDisk(disk),
		)
	}

	for index, iface := range cfg.Interfaces {
		validationErr := validateInterface(iface)
		if validationErr != nil {
			return nil, errs.Wrap(
				errors.Join(errs.ErrConfig, errs.ErrInterface),
				fmt.Sprintf("interface %d", index),
				validationErr,
			)
		}

		d.Devices.Interfaces = append(
			d.Devices.Interfaces,
			buildInterface(iface),
		)
	}

	return d, nil
}

func buildDisk(disk domain.RuntimeDisk) libvirtxml.DomainDisk {
	format := disk.Format
	if format == "" {
		format = string(domain.DiskFormatRaw)
	}

	bus := disk.Bus
	if bus == "" {
		bus = string(domain.DiskBusVirtio)
	}

	target := disk.Target
	if target == "" {
		target = "vda"
	}

	d := libvirtxml.DomainDisk{
		Device: "disk",

		Driver: &libvirtxml.DomainDiskDriver{
			Name: "qemu",
			Type: format,
		},

		Source: &libvirtxml.DomainDiskSource{
			File: &libvirtxml.DomainDiskSourceFile{
				File: disk.Source,
			},
		},

		Target: &libvirtxml.DomainDiskTarget{
			Dev: target,
			Bus: bus,
		},
	}

	if disk.ReadOnly {
		d.ReadOnly = &libvirtxml.DomainDiskReadOnly{}
	}

	return d
}

func buildInterface(iface domain.Interface) libvirtxml.DomainInterface {
	model := iface.Model
	if model == "" {
		model = domain.InterfaceModelVirtio
	}

	i := libvirtxml.DomainInterface{
		Source: &libvirtxml.DomainInterfaceSource{
			Bridge: &libvirtxml.DomainInterfaceSourceBridge{
				Bridge: iface.Bridge,
			},
		},

		Model: &libvirtxml.DomainInterfaceModel{
			Type: string(model),
		},
	}

	if iface.MAC != "" {
		i.MAC = &libvirtxml.DomainInterfaceMAC{
			Address: iface.MAC,
		}
	}

	return i
}

func validateDisk(disk domain.RuntimeDisk) error {
	if disk.Source == "" {
		return errs.New(errs.ErrDiskSource, "")
	}

	switch disk.Format {
	case "", string(domain.DiskFormatRaw), string(domain.DiskFormatQCOW2):
	default:
		return errs.New(errs.ErrDiskFormat, disk.Format)
	}

	switch disk.Bus {
	case "",
		string(domain.DiskBusVirtio),
		string(domain.DiskBusSATA),
		string(domain.DiskBusSCSI),
		string(domain.DiskBusIDE),
		string(domain.DiskBusUSB):
	default:
		return errs.New(errs.ErrDiskBus, disk.Bus)
	}

	return nil
}

func validateInterface(iface domain.Interface) error {
	if iface.Bridge == "" {
		return errs.New(errs.ErrInterfaceBridge, "")
	}

	switch iface.Model {
	case "",
		domain.InterfaceModelVirtio,
		domain.InterfaceModelE1000:
	default:
		return errs.New(
			errs.ErrInterfaceModel,
			string(iface.Model),
		)
	}

	if iface.MAC != "" {
		if _, parseErr := net.ParseMAC(iface.MAC); parseErr != nil {
			return errs.Wrap(
				errs.ErrInterfaceMAC,
				iface.MAC,
				parseErr,
			)
		}
	}

	return nil
}
