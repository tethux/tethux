package container

import "github.com/tethux/tethux/internal/libtethux/virt"

type ContainerNode struct {
	virt.Node

	PID       uint32
	ImageID   string
	ImageName string

	Labels   map[string]string
	Networks []string
}
