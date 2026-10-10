package topology

import (
	"context"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/local"
	"github.com/tethux/tethux/virt/container"
)

func logSummary(ctx context.Context, logger *slog.Logger, top *topology.Topology, run *local.Running, provider container.ContainerProvider) error {
	for _, node := range top.Nodes {
		id, err := run.ContainerID(node.ID)
		if err != nil {
			return err
		}
		address := discoverAddress(ctx, provider, id)
		if address.IsValid() {
			logger.InfoContext(ctx, "Topology node", "node", node.ID, "container", id, "ipv4", address.String())
		} else {
			logger.InfoContext(ctx, "Topology node", "node", node.ID, "container", id)
		}
	}
	for _, link := range top.Links {
		logger.InfoContext(ctx, "Topology link", "link", link.ID, "a", link.A.String(), "b", link.B.String())
	}
	return nil
}

// Guest setup is asynchronous; address discovery remains optional and bounded.
func discoverAddress(ctx context.Context, provider container.ContainerProvider, id string) netip.Addr {
	probeCtx, cancel := context.WithTimeout(ctx, 600*time.Millisecond)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		stdout, stderr, err := provider.Exec(probeCtx, id, []string{"ip", "-o", "-4", "addr", "show", "scope", "global"}, nil, nil)
		if err != nil || len(stderr) != 0 {
			return netip.Addr{}
		}
		address := firstAddress(string(stdout))
		if address.IsValid() {
			return address
		}
		select {
		case <-probeCtx.Done():
			return netip.Addr{}
		case <-ticker.C:
		}
	}
}

func firstAddress(output string) netip.Addr {
	fields := strings.Fields(output)
	for index, field := range fields {
		if field != "inet" || index+1 >= len(fields) {
			continue
		}
		prefix, err := netip.ParsePrefix(fields[index+1])
		if err == nil && prefix.Addr().Is4() && !prefix.Addr().IsLoopback() {
			return prefix.Addr()
		}
	}
	return netip.Addr{}
}
