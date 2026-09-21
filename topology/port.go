package topology

// PortID identifies a port within its node.
type PortID string

// PortKind identifies the medium supported by a logical port.
type PortKind string

const (
	// PortEthernet selects the ethernet medium.
	PortEthernet PortKind = "ethernet"
	// PortSerial selects the serial medium.
	PortSerial PortKind = "serial"
)

// Port describes a logical attachment point independent of host interfaces.
type Port struct {
	ID   PortID
	Kind PortKind
}

// Endpoint identifies a port by its node and local port ID.
type Endpoint struct {
	Node NodeID
	Port PortID
}

// String returns a human-readable node/port label.
func (e Endpoint) String() string {
	return string(e.Node) + "/" + string(e.Port)
}
