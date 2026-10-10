package local

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/google/uuid"
	mobyclient "github.com/moby/moby/client"
	"github.com/tethux/tethux/bridge"
	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/container"
)

// Options selects host resources without changing the declarative topology.
type Options struct {
	Provider container.ContainerProvider
	// Logger receives lifecycle progress. Nil disables logging.
	Logger *slog.Logger
	// BasePort starts consecutive loopback UDP port pairs. Zero selects 23000.
	BasePort int
}

// Running owns the containers and bridges created by Start.
// Call Close with a live context when finished. Methods are not concurrent-safe.
type Running struct {
	provider container.ContainerProvider
	logger   *slog.Logger
	nodes    map[topology.NodeID]string
	order    []topology.NodeID
	bridges  []*bridge.ContainerBridge
}

// Start creates network-isolated containers and connects the declared Ethernet links.
// Missing images are pulled through the provider. Only container specs and
// Ethernet ports are supported. Linked port IDs become container interface names.
// Unlinked ports are left unattached. The caller needs namespace and raw-socket
// privileges on the host running the containers. A failed start rolls back its resources.
func Start(ctx context.Context, top *topology.Topology, options Options) (*Running, error) {
	err := top.Validate()
	if err != nil {
		return nil, err
	}
	err = validateSupport(top, options)
	if err != nil {
		return nil, err
	}
	if options.BasePort == 0 {
		options.BasePort = 23000
	}
	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}
	run := &Running{
		provider: options.Provider,
		logger:   options.Logger,
		nodes:    make(map[topology.NodeID]string, len(top.Nodes)),
		order:    make([]topology.NodeID, 0, len(top.Nodes)),
		bridges:  make([]*bridge.ContainerBridge, 0, 2*len(top.Links)),
	}
	token := strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	err = run.start(ctx, top, options.BasePort, token)
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		run.logger.WarnContext(cleanupCtx, "Rolling back topology startup", "error", err)
		cleanupErr := run.Close(cleanupCtx)
		return nil, errors.Join(errs.Wrap("start local topology", errs.ErrStart, string(top.ID), err), cleanupErr)
	}
	return run, nil
}

func validateSupport(top *topology.Topology, options Options) error {
	if options.Provider == nil {
		return errs.New("start local topology", errs.ErrStart, "container provider is required")
	}
	basePort := options.BasePort
	if basePort == 0 {
		basePort = 23000
	}
	if basePort < 1 || basePort > 65535 || len(top.Links) > (65536-basePort)/2 {
		return errs.New("start local topology", errs.ErrStart, "UDP port range must fit within 1..65535")
	}
	for _, node := range top.Nodes {
		if _, ok := node.Spec.(topology.ContainerSpec); !ok {
			return errs.New("start local topology", errs.ErrUnsupported, fmt.Sprintf("node %q is not a container", node.ID))
		}
		for _, port := range node.Ports {
			if port.Kind != topology.PortEthernet {
				return errs.New("start local topology", errs.ErrUnsupported, fmt.Sprintf("node %q port %q is not Ethernet", node.ID, port.ID))
			}
			name := string(port.ID)
			if len(name) > 15 || name == "." || name == ".." || name == "lo" || strings.ContainsAny(name, "/:\x00 \t\r\n") {
				return errs.New("start local topology", errs.ErrUnsupported, fmt.Sprintf("port %q must be a Linux interface name other than lo", port.ID))
			}
		}
	}
	for _, link := range top.Links {
		if link.Kind != topology.LinkEthernet || (link.MTU != 0 && link.MTU < 68) {
			return errs.New("start local topology", errs.ErrUnsupported, fmt.Sprintf("link %q requires Ethernet and MTU >= 68", link.ID))
		}
	}
	return nil
}

func (run *Running) start(ctx context.Context, top *topology.Topology, basePort int, token string) error {
	pids := make(map[topology.NodeID]int, len(top.Nodes))
	for index, node := range top.Nodes {
		spec, ok := node.Spec.(topology.ContainerSpec)
		if !ok {
			return errs.New("start local topology", errs.ErrUnsupported, string(node.ID))
		}
		env := make([]string, 0, len(spec.Env))
		for _, variable := range spec.Env {
			env = append(env, variable.Name+"="+variable.Value)
		}
		config := &container.RuntimeConfig{
			NodeConfig: virt.NodeConfig{Name: fmt.Sprintf("tethux-topology-%s-%d", token, index)},
			Image:      container.ParseImage(spec.Image), Cmd: spec.Command, Env: env,
			NetworkMode: "none", CapAdd: []string{"CAP_NET_ADMIN", "CAP_NET_RAW"},
			Labels: map[string]string{"tethux.topology": string(top.ID), "tethux.node": string(node.ID)},
		}
		run.logger.InfoContext(ctx, "Creating container", "node", node.ID, "image", spec.Image)
		created, err := run.provider.CreateContainer(ctx, config)
		if errors.Is(err, errdefs.ErrNotFound) {
			run.logger.InfoContext(ctx, "Pulling image", "node", node.ID, "image", spec.Image)
			err = run.provider.Pull(ctx, spec.Image, nil)
			if err != nil {
				return err
			}
			created, err = run.provider.CreateContainer(ctx, config)
		}
		if err != nil {
			return err
		}
		run.nodes[node.ID] = created.ID
		run.order = append(run.order, node.ID)
		err = run.provider.Start(ctx, created.ID)
		if err != nil {
			return err
		}
		inspected, err := run.provider.Inspect(ctx, created.ID, nil)
		if err != nil {
			return err
		}
		if inspected.State != virt.NodeRunning || inspected.PID == 0 {
			return errs.New("start local topology", errs.ErrStart, fmt.Sprintf("node %q did not enter running state", node.ID))
		}
		pids[node.ID] = int(inspected.PID)
		run.logger.InfoContext(ctx, "Container running", "node", node.ID, "container", created.ID[:min(12, len(created.ID))])
	}
	for index, link := range top.Links {
		run.logger.InfoContext(ctx, "Connecting link", "link", link.ID, "a", link.A.String(), "b", link.B.String())
		mtu := int(link.MTU)
		if mtu == 0 {
			mtu = 1500
		}
		for side, endpoint := range []topology.Endpoint{link.A, link.B} {
			err := ctx.Err()
			if err != nil {
				return err
			}
			port := basePort + 2*index + side
			remote := basePort + 2*index + 1 - side
			connection, err := bridge.StartContainerBridge(&bridge.ContainerBridgeOptions{
				PID: pids[endpoint.Node], HostIf: fmt.Sprintf("tx%s%04x", token, 2*index+side),
				ContainerIf: string(endpoint.Port), MTU: mtu,
				Listen: fmt.Sprintf("127.0.0.1:%d", port), Remote: fmt.Sprintf("127.0.0.1:%d", remote),
			})
			if err != nil {
				return err
			}
			run.bridges = append(run.bridges, connection)
		}
	}
	return nil
}

// ContainerID resolves a topology node to its provider's runtime identity.
func (run *Running) ContainerID(node topology.NodeID) (string, error) {
	id, exists := run.nodes[node]
	if !exists {
		return "", errs.New("find local topology node", errs.ErrNotFound, string(node))
	}
	return id, nil
}

// Close stops bridges and deletes this run's containers, including running ones.
// It is safe to call again; failed deletions remain available for retry.
func (run *Running) Close(ctx context.Context) error {
	if run == nil {
		return nil
	}
	var failures []error
	for index := len(run.bridges) - 1; index >= 0; index-- {
		err := run.bridges[index].Close()
		if err != nil {
			failures = append(failures, errs.Wrap("close local topology bridge", errs.ErrCleanup, "", err))
		}
	}
	run.bridges = nil
	for index := len(run.order) - 1; index >= 0; index-- {
		node := run.order[index]
		id, exists := run.nodes[node]
		if !exists {
			continue
		}
		run.logger.InfoContext(ctx, "Removing container", "node", node)
		err := run.provider.DeleteContainer(ctx, id, &mobyclient.ContainerRemoveOptions{Force: true})
		if err != nil {
			failures = append(failures, errs.Wrap("delete local topology node", errs.ErrCleanup, string(node), err))
			continue
		}
		delete(run.nodes, node)
	}
	return errors.Join(failures...)
}
