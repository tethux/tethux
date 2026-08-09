package container

import "github.com/tethux/tethux/virt"

// ContainerNode describes a container and its provider-specific runtime state.
type ContainerNode struct {
	virt.Node

	PID       uint32
	ImageID   string
	ImageName string

	Labels   map[string]string
	Networks []string
}
