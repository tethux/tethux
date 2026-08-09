package virt

import "context"

// ProviderKind classifies the workload technology managed by a provider.
type ProviderKind string

const (
	// ProviderKindContainer identifies an OCI container provider.
	ProviderKindContainer ProviderKind = "container"
	// ProviderKindDomain identifies a virtual-machine provider.
	ProviderKindDomain ProviderKind = "domain"
	// ProviderKindEmulator identifies an emulated-device provider.
	ProviderKindEmulator ProviderKind = "emulator"
)

// Capabilities declares optional operations supported by a provider.
type Capabilities struct {
	Console       bool
	AuxConsole    bool
	Exec          bool
	Logs          bool
	Pause         bool
	Snapshots     bool
	ManagedServer bool
}

// ProviderInfo describes a provider and its supported operations.
type ProviderInfo struct {
	Name         string
	DisplayName  string
	Kind         ProviderKind
	Capabilities Capabilities
}

// ConsoleType identifies a workload console transport.
type ConsoleType string

const (
	// ConsoleNone indicates that no console is available.
	ConsoleNone ConsoleType = "none"
	// ConsoleTelnet identifies a Telnet console.
	ConsoleTelnet ConsoleType = "telnet"
	// ConsoleSerial identifies a serial console.
	ConsoleSerial ConsoleType = "serial"
	// ConsoleVNC identifies a VNC console.
	ConsoleVNC ConsoleType = "vnc"
	// ConsoleSpice identifies a SPICE console.
	ConsoleSpice ConsoleType = "spice"
	// ConsoleAux identifies an auxiliary console.
	ConsoleAux ConsoleType = "aux"
)

// Console describes how to connect to a workload console.
type Console struct {
	Type ConsoleType
	Host string
	Port uint16
}

// NodeState describes a workload's lifecycle state.
type NodeState string

const (
	// NodeStopped indicates that the workload is not running.
	NodeStopped NodeState = "stopped"
	// NodeStarting indicates that the workload is starting.
	NodeStarting NodeState = "starting"
	// NodeRunning indicates that the workload is running.
	NodeRunning NodeState = "running"
	// NodeStopping indicates that the workload is stopping.
	NodeStopping NodeState = "stopping"
	// NodeSuspended indicates that the workload is paused.
	NodeSuspended NodeState = "suspended"
)

// NodeConfig contains provider-independent workload configuration.
type NodeConfig struct {
	ID       string
	Name     string
	Image    string
	CPUs     int
	MemoryMB int

	ConsoleType    ConsoleType
	AuxConsoleType ConsoleType

	Meta map[string]string
}

// Node describes a workload managed by a Provider.
type Node struct {
	ID      string
	Name    string
	State   NodeState
	Console Console
	Aux     *Console
}

// Provider manages the lifecycle and state of virtualized workloads.
type Provider interface {
	Info() ProviderInfo

	Start(ctx context.Context, id string) error
	Stop(ctx context.Context, id string) error
	Suspend(ctx context.Context, id string) error
	Resume(ctx context.Context, id string) error
	Restart(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error

	State(ctx context.Context, id string) (NodeState, error)
	Reload(ctx context.Context, id string) (*Node, error)
	List(ctx context.Context) ([]*Node, error)
}
