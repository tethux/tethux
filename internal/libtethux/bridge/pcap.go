package bridge

import (
	"github.com/google/gopacket/pcap"
	"github.com/tethux/tethux/internal/assert"
)

// PcapPort transports Ethernet frames through a pcap interface handle.
type PcapPort struct {
	id     string
	mtu    int
	ifName string
	handle *pcap.Handle
}

// ID returns the stable port identifier.
func (p *PcapPort) ID() string {
	if assert.Enabled {
		p.assertValid()
	}
	return p.id
}

// MTU returns the configured frame MTU.
func (p *PcapPort) MTU() int {
	if assert.Enabled {
		p.assertValid()
	}
	return p.mtu
}

// ReadFrame reads one Ethernet frame from the pcap handle.
func (p *PcapPort) ReadFrame() (Frame, error) {
	if assert.Enabled {
		p.assertValid()
	}
	data, _, err := p.handle.ReadPacketData()
	return data, err
}

// WriteFrame writes one Ethernet frame through the pcap handle.
func (p *PcapPort) WriteFrame(frame Frame) error {
	if assert.Enabled {
		p.assertValid()
	}
	assert.That(frame != nil, "PcapPort %q writing nil frame", p.id)
	return p.handle.WritePacketData(frame)
}

// Close closes the pcap handle.
func (p *PcapPort) Close() error {
	if assert.Enabled {
		p.assertValid()
	}
	p.handle.Close()
	return nil
}

func (p *PcapPort) assertValid() {
	if !assert.Enabled {
		return
	}
	assert.That(p != nil, "nil PcapPort")
	assert.That(p.id != "", "PcapPort has empty ID")
	assert.That(p.mtu > 0, "PcapPort %q has invalid MTU %d", p.id, p.mtu)
	assert.That(p.ifName != "", "PcapPort %q has empty interface name", p.id)
	assert.That(p.handle != nil, "PcapPort %q has nil handle", p.id)
}
