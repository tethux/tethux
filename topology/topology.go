package topology

// ID identifies a topology.
type ID string

// Topology describes workloads, connections, and optional visual layouts.
type Topology struct {
	ID    ID
	Nodes []Node
	Links []Link
	Views []View
}
