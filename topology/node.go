package topology

// NodeID identifies a workload within a topology.
type NodeID string

// NodeKind identifies a workload spec family.
type NodeKind string

const (
	// NodeKindContainer identifies an OCI container workload.
	NodeKindContainer NodeKind = "container"
	// NodeKindDomain identifies a virtual machine workload.
	NodeKindDomain NodeKind = "domain"
	// NodeKindShitnet identifies a future shitnet provider workload.
	NodeKindShitnet NodeKind = "shitnet"
	// NodeKindDynamips identifies a Dynamips router workload.
	NodeKindDynamips NodeKind = "dynamips"
)

// Node describes a workload and its logical ports.
type Node struct {
	ID    NodeID
	Name  string
	Spec  NodeSpec
	Ports []Port
}

// NodeSpec is a closed set of provider-independent workload specs.
// Store concrete spec values in Node.Spec.
type NodeSpec interface {
	nodeSpec()
	Kind() NodeKind
}

// ContainerSpec describes an OCI workload.
type ContainerSpec struct {
	Image   string
	Command []string
	Env     []Env
}

func (ContainerSpec) nodeSpec() {}

// Kind returns the container node kind.
func (ContainerSpec) Kind() NodeKind {
	return NodeKindContainer
}

// Env describes one container environment variable.
type Env struct {
	Name  string
	Value string
}

// DomainSpec describes a virtual machine image and optional resource sizes.
// Zero resource sizes leave sizing to the provider.
type DomainSpec struct {
	Image    string
	CPUs     uint16
	MemoryMB uint32
}

func (DomainSpec) nodeSpec() {}

// Kind returns the domain node kind.
func (DomainSpec) Kind() NodeKind {
	return NodeKindDomain
}

// ShitnetSpec reserves a node kind for the future provider.
type ShitnetSpec struct {
	// Whenever i add the provider
}

func (ShitnetSpec) nodeSpec() {}

// Kind returns the shitnet node kind.
func (ShitnetSpec) Kind() NodeKind {
	return NodeKindShitnet
}

// DynamipsSpec describes a router image and optional RAM size in MiB.
type DynamipsSpec struct {
	Image string
	RAMMB uint32
}

func (DynamipsSpec) nodeSpec() {}

// Kind returns the dynamips node kind.
func (DynamipsSpec) Kind() NodeKind {
	return NodeKindDynamips
}
