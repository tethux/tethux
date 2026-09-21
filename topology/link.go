package topology

// LinkID identifies a connection within a topology.
type LinkID string

// LinkKind identifies the medium used by a connection.
type LinkKind string

const (
	// LinkEthernet selects the ethernet medium.
	LinkEthernet LinkKind = "ethernet"
	// LinkSerial selects the serial medium.
	LinkSerial LinkKind = "serial"
)

// Link connects two distinct ports of the same medium.
// MTU is optional; zero leaves it unspecified.
type Link struct {
	ID   LinkID
	Kind LinkKind

	A Endpoint
	B Endpoint

	MTU uint16
}
