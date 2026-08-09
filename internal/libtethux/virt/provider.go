package virt

import "context"

type ProviderKind string

const (
	ProviderKindContainer ProviderKind = "container"
	ProviderKindDomain    ProviderKind = "domain"
	ProviderKindEmulator  ProviderKind = "emulator"
)

type Capabilities struct {
	Console       bool
	AuxConsole    bool
	Exec          bool
	Logs          bool
	Pause         bool
	Snapshots     bool
	ManagedServer bool
}

type ProviderInfo struct {
	Name         string
	DisplayName  string
	Kind         ProviderKind
	Capabilities Capabilities
}

type ConsoleType string

const (
	ConsoleNone   ConsoleType = "none"
	ConsoleTelnet ConsoleType = "telnet"
	ConsoleSerial ConsoleType = "serial"
	ConsoleVNC    ConsoleType = "vnc"
	ConsoleSpice  ConsoleType = "spice"
	ConsoleAux    ConsoleType = "aux"
)

type Console struct {
	Type ConsoleType
	Host string
	Port uint16
}

type NodeState string

const (
	NodeStopped   NodeState = "stopped"
	NodeStarting  NodeState = "starting"
	NodeRunning   NodeState = "running"
	NodeStopping  NodeState = "stopping"
	NodeSuspended NodeState = "suspended"
)

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

type Node struct {
	ID      string
	Name    string
	State   NodeState
	Console Console
	Aux     *Console
}

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
