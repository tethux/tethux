package topology

// ViewID identifies a visual layout within a topology.
type ViewID string

// View describes an optional layout of a subset of topology resources.
type View struct {
	ID    ViewID
	Name  string
	Nodes []NodeView
	Links []LinkView
}

// NodeView places a node in a view.
type NodeView struct {
	Node     NodeID
	Position Position
}

// LinkView describes optional routing points for a link in a view.
type LinkView struct {
	Link   LinkID
	Points []Position
}

// Position is a finite coordinate in a view.
type Position struct {
	X float64
	Y float64
}
