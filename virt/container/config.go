package container

import (
	"net/netip"
	"strings"

	"github.com/tethux/tethux/storage"
	"github.com/tethux/tethux/virt"
)

// ContainerConfig describes a provider-independent OCI container workload.
type ContainerConfig struct {
	virt.NodeConfig

	Image Image

	Entrypoint []string
	Cmd        []string
	Env        []string

	Volumes []VolumeMount

	CapAdd      []string
	CapDrop     []string
	Privileged  bool
	NetworkMode string

	Hostname   string
	DNS        []netip.Addr
	ExtraHosts []string

	Labels map[string]string
}

// Image is the persistent, provider-independent identity of an OCI image.
// Registry contains the repository path (and may include a registry host).
type Image struct {
	Registry string
	Tag      string
	Digest   string
}

// ParseImage parses an OCI image reference into repository, tag, and digest parts.
func ParseImage(ref string) Image {
	image := Image{}
	image.Registry, image.Digest, _ = strings.Cut(ref, "@")
	if image.Digest != "" {
		return image
	}

	lastSlash := strings.LastIndexByte(image.Registry, '/')
	if colon := strings.LastIndexByte(image.Registry, ':'); colon > lastSlash {
		image.Registry, image.Tag = image.Registry[:colon], image.Registry[colon+1:]
	}
	return image
}

// String reconstructs the image reference.
func (i Image) String() string {
	if i.Digest != "" {
		if i.Registry == "" {
			return i.Digest
		}
		return i.Registry + "@" + i.Digest
	}
	if i.Tag != "" {
		if i.Registry == "" {
			return i.Tag
		}
		return i.Registry + ":" + i.Tag
	}
	return i.Registry
}

// VolumeMount maps a storage reference into a container.
type VolumeMount struct {
	Source storage.Ref
	Target string

	ReadOnly bool
}
