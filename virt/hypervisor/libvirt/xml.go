package libvirt

import (
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/tethux/tethux/virt/domain"
	"github.com/tethux/tethux/virt/hypervisor/libvirt/errs"
)

// i will soon nuke all of this and make a better soltuion

func domainXML(cfg *domain.RuntimeConfig) (string, error) {
	if cfg == nil {
		return "", errs.New(errs.ErrConfig, "nil config")
	}
	name := cfg.Name
	if name == "" {
		name = cfg.ID
	}
	if name == "" {
		return "", errs.New(errs.ErrConfig, "name is empty")
	}

	var b strings.Builder
	write := func(format string, values ...string) {
		for _, value := range values {
			escaped := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(value)
			format = strings.Replace(format, "%s", escaped, 1)
		}
		b.WriteString(format)
	}
	write(`<domain type="kvm"><name>%s</name>`, name)
	if parsed, err := uuid.Parse(cfg.ID); err == nil {
		write(`<uuid>%s</uuid>`, parsed.String())
	}
	if cfg.MemoryMB > 0 {
		write(`<memory unit="MiB">%s</memory>`, strconv.Itoa(cfg.MemoryMB))
	}
	if cfg.CPUs > 0 {
		write(`<vcpu>%s</vcpu>`, strconv.Itoa(cfg.CPUs))
	}
	arch := cfg.Architecture
	if arch == "" {
		arch = "x86_64"
	}
	machine := cfg.Machine
	if machine == "" {
		machine = "q35"
	}
	write(`<os><type arch="%s" machine="%s">hvm</type></os><devices>`, arch, machine)
	for _, disk := range cfg.Disks {
		format := disk.Format
		if format == "" {
			format = "raw"
		}
		bus := disk.Bus
		if bus == "" {
			bus = "virtio"
		}
		target := disk.Target
		if target == "" {
			target = "vda"
		}
		write(`<disk type="file" device="disk"><driver name="qemu" type="%s"/><source file="%s"/><target dev="%s" bus="%s"/>`, format, disk.Source, target, bus)
		if disk.ReadOnly {
			b.WriteString(`<readonly/>`)
		}
		b.WriteString(`</disk>`)
	}
	for _, iface := range cfg.Interfaces {
		bridge := iface.Bridge
		if bridge == "" {
			continue
		}
		model := iface.Model
		if model == "" {
			model = "virtio"
		}
		write(`<interface type="bridge"><source bridge="%s"/><model type="%s"/>`, bridge, model)
		if iface.MAC != "" {
			write(`<mac address="%s"/>`, iface.MAC)
		}
		b.WriteString(`</interface>`)
	}
	b.WriteString(`<serial type="pty"/><console type="pty"/><graphics type="spice" autoport="yes" listen="127.0.0.1"/></devices></domain>`)
	return b.String(), nil
}
