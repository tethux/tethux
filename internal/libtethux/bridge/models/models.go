package models

// LinkState describes the lifecycle state of a network link.
type LinkState string

const (
	// Up indicates that a link is operational.
	Up LinkState = "Up"
	// Down indicates that a link is not operational.
	Down LinkState = "Down"
	// Error indicates that a link operation failed.
	Error LinkState = "Error"
)

// Link describes a named network link and its configuration.
type Link struct {
	ID        string
	SourcePID int
	TargetPID int
	MTU       int
	State     LinkState
}

// NamespaceInterfaceMode selects how a namespace interface is prepared.
type NamespaceInterfaceMode string

const (
	// NamespaceInterfaceCreateVeth creates a new veth pair for the namespace.
	NamespaceInterfaceCreateVeth NamespaceInterfaceMode = "create-veth"
	// NamespaceInterfaceExisting prepares an existing host interface.
	NamespaceInterfaceExisting NamespaceInterfaceMode = "existing"
)

// SetupLinkParams configures creation of a veth pair for a process namespace.
type SetupLinkParams struct {
	SourcePID int
	HostName  string
	Container string
	MTU       int
}
