package topology

import (
	"context"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/local"
	"github.com/tethux/tethux/virt/container"
)

type nodeSummary struct {
	node    topology.NodeID
	id      string
	address netip.Addr
}

func printSummary(ctx context.Context, output io.Writer, top *topology.Topology, run *local.Running, provider container.ContainerProvider) error {
	nodes := make([]nodeSummary, 0, len(top.Nodes))
	for _, node := range top.Nodes {
		id, err := run.ContainerID(node.ID)
		if err != nil {
			return err
		}
		address := discoverAddress(ctx, provider, id)
		nodes = append(nodes, nodeSummary{node: node.ID, id: id, address: address})
	}
	var text strings.Builder
	table := tabwriter.NewWriter(&text, 0, 4, 2, ' ', 0)
	fmt.Fprintf(table, "NODE\tCONTAINER\tIPv4\n")
	for _, node := range nodes {
		address := "-"
		if node.address.IsValid() {
			address = node.address.String()
		}
		fmt.Fprintf(table, "%s\t%s\t%s\n", node.node, node.id[:min(12, len(node.id))], address)
	}
	err := table.Flush()
	if err != nil {
		return err
	}
	fmt.Fprintf(&text, "\nLinks:\n")
	for _, link := range top.Links {
		// #nosec G705 -- this builds plain text for the CLI terminal.
		fmt.Fprintf(&text, "  %s: %s <-> %s\n", link.ID, link.A, link.B)
	}
	fmt.Fprintf(&text, "\nCommands to run in another terminal:\n")
	runtime := provider.Info().Name
	for _, node := range nodes {
		fmt.Fprintf(&text, "\n  # %s\n", node.node)
		fmt.Fprintf(&text, "  %s exec -it %s sh\n", runtime, shellArg(node.id[:min(12, len(node.id))]))
		fmt.Fprintf(&text, "  %s exec %s ip -br addr\n", runtime, shellArg(node.id[:min(12, len(node.id))]))
		for _, peer := range nodes {
			if node.address.IsValid() && peer.node != node.node && peer.address.IsValid() {
				fmt.Fprintf(&text, "  %s exec %s ping -c 3 %s\n", runtime, shellArg(node.id[:min(12, len(node.id))]), peer.address)
				break
			}
		}
	}
	_, err = io.WriteString(output, text.String())
	return err
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

func shellArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
