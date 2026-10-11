package emulator

import "github.com/tethux/tethux/virt"

// Node describes an emulated device and its runtime state.
type Node struct {
	virt.Node
}
