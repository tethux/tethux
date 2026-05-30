package libtethux

import "github.com/google/gopacket/pcap"

type PcapPort struct {
	id     string
	mtu    int
	ifName string
	handle *pcap.Handle
}

func (p *PcapPort) ID() string {
	return p.id
}

func (p *PcapPort) MTU() int {
	return p.mtu
}

func (p *PcapPort) ReadFrame() (Frame, error) {
	data, _, err := p.handle.ReadPacketData()
	return data, err
}

func (p *PcapPort) WriteFrame(frame Frame) error {
	return p.handle.WritePacketData(frame)
}

func (p *PcapPort) Close() error {
	p.handle.Close()
	return nil
}
