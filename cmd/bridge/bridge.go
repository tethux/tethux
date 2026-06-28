package bridge

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	libtethux_br "github.com/0xveya/tethux/internal/libtethux/bridge"
	"github.com/0xveya/tethux/internal/libtethux/bridge/models"
	"github.com/spf13/cobra"
)

type portSpec struct {
	ID            string
	Scheme        libtethux_br.AvailableScheme
	Interface     string
	Listen        string
	Remote        string
	MTU           int
	ImmediateMode bool
	SnapLen       int
	Latency       time.Duration
}

func newBridgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bridge",
		Short: "Run the switch with different transport backends",
	}

	cmd.AddCommand(newBridgePortsCmd())
	cmd.AddCommand(newBridgeContainerCmd())
	cmd.AddCommand(newBridgeNamespaceCmd())
	cmd.AddCommand(newBridgeUDPCmd())

	return cmd
}

func newBridgePortsCmd() *cobra.Command {
	var (
		specs                      []string
		disableUnknownUnicastFlood bool
	)

	cmd := &cobra.Command{
		Use:   "ports",
		Short: "Run a switch with repeated transport port specs",
		Long: "Each --port spec is a comma-separated key=value list.\n" +
			"Examples:\n" +
			"  --port id=left,scheme=udp,listen=127.0.0.1:10001,remote=127.0.0.1:11001\n" +
			"  --port id=tap0,scheme=tap,if=tap0\n" +
			"  --port id=uplink,scheme=raw,if=tx0\n" +
			"  --port id=capture,scheme=pcap,if=enp0s1,snaplen=1532,immediate=true",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(specs) < 2 {
				return fmt.Errorf("need at least two --port specs")
			}

			ports, err := parsePortSpecs(specs)
			if err != nil {
				return err
			}

			sw := libtethux_br.NewSwitch(libtethux_br.SwitchOptions{
				DisableUnknownUnicastFlood: disableUnknownUnicastFlood,
			})

			attached, err := attachSwitchPorts(sw, ports)
			if err != nil {
				closePorts(attached)
				return err
			}

			printPortSummary("Switch ports", ports)

			return runSwitch(sw)
		},
	}

	cmd.Flags().StringArrayVar(&specs, "port", nil, "port spec: id=<name>,scheme=<udp|raw|pcap>,...")
	cmd.Flags().BoolVar(&disableUnknownUnicastFlood, "disable-unknown-unicast-flood", false, "drop unknown unicast instead of flooding")

	return cmd
}

func newBridgeContainerCmd() *cobra.Command {
	var (
		pid                        int
		hostIf                     string
		containerIf                string
		specs                      []string
		mtu                        int
		usePcap                    bool
		immediate                  bool
		disableUnknownUnicastFlood bool
		interfaceMode              string
	)

	cmd := &cobra.Command{
		Use:   "container",
		Short: "Attach one namespace interface and optional UDP links to a switch",
		Long: "By default, tethux creates a veth pair in Go, moves one side into the target namespace, " +
			"then runs a switch with the host side plus any repeated UDP --port specs. " +
			"Use --interface-mode=existing when another runtime already prepared the host interface.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pid <= 0 {
				return fmt.Errorf("--pid must be a running container or namespace pid")
			}
			if hostIf == "" {
				return fmt.Errorf("--host-if is required")
			}
			if containerIf == "" {
				return fmt.Errorf("--container-if is required")
			}

			udpPorts, err := parsePortSpecs(specs)
			if err != nil {
				return err
			}
			for _, spec := range udpPorts {
				if spec.Scheme != libtethux_br.UDPScheme {
					return fmt.Errorf("container bridge --port only accepts scheme=udp, got %s for %s", spec.Scheme, spec.ID)
				}
			}

			scheme := libtethux_br.RawScheme
			if usePcap {
				scheme = libtethux_br.PcapScheme
			}

			sw := libtethux_br.NewSwitch(libtethux_br.SwitchOptions{
				DisableUnknownUnicastFlood: disableUnknownUnicastFlood,
			})

			mode := models.NamespaceInterfaceMode(interfaceMode)
			createVeth := mode == "" || mode == models.NamespaceInterfaceCreateVeth

			if createVeth {
				libtethux_br.CleanupLink(hostIf)
			}
			defer func() {
				if stopErr := sw.Stop(); stopErr != nil {
					log.Printf("switch shutdown error: %v", stopErr)
				}
				if createVeth {
					libtethux_br.CleanupLink(hostIf)
				}
			}()

			attachErr := libtethux_br.AttachNamespaceInterface(libtethux_br.NamespaceInterfaceOptions{
				Mode:              mode,
				PID:               pid,
				HostSideName:      hostIf,
				ContainerSideName: containerIf,
				MTU:               mtu,
			})
			if attachErr != nil {
				return fmt.Errorf("prepare %s with mode %s for pid %d as %s: %w", hostIf, mode, pid, containerIf, attachErr)
			}

			ports := append([]portSpec{{
				ID:            "container",
				Scheme:        scheme,
				Interface:     hostIf,
				MTU:           mtu,
				ImmediateMode: immediate,
				SnapLen:       mtu + 32,
			}}, udpPorts...)

			attached, err := attachSwitchPorts(sw, ports)
			if err != nil {
				closePorts(attached)
				return err
			}

			printPortSummary("Container bridge", ports)

			return runSwitch(sw)
		},
	}

	cmd.Flags().IntVar(&pid, "pid", 0, "target namespace pid")
	cmd.Flags().StringVar(&hostIf, "host-if", "", "host-side veth name")
	cmd.Flags().StringVar(&containerIf, "container-if", "tx0", "interface name inside the namespace")
	cmd.Flags().StringArrayVar(&specs, "port", nil, "UDP port spec: id=<name>,scheme=udp,listen=<addr>,remote=<addr>")
	cmd.Flags().IntVar(&mtu, "mtu", 1500, "link MTU")
	cmd.Flags().BoolVar(&usePcap, "pcap", false, "use pcap instead of raw sockets for the host-side veth")
	cmd.Flags().BoolVar(&immediate, "immediate", true, "enable pcap immediate mode")
	cmd.Flags().BoolVar(&disableUnknownUnicastFlood, "disable-unknown-unicast-flood", false, "drop unknown unicast instead of flooding")
	cmd.Flags().StringVar(&interfaceMode, "interface-mode", string(models.NamespaceInterfaceCreateVeth), "namespace interface mode: create-veth or existing")

	return cmd
}

func newBridgeNamespaceCmd() *cobra.Command {
	var (
		usePcap     bool
		immediate   bool
		hostA       string
		hostB       string
		containerIf string
		mtu         int
	)

	cmd := &cobra.Command{
		Use:   "namespace <pid-a> <pid-b>",
		Short: "Bridge two Linux namespaces through veth pairs",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pidA, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid pid-a: %w", err)
			}

			pidB, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid pid-b: %w", err)
			}

			scheme := libtethux_br.RawScheme
			if usePcap {
				scheme = libtethux_br.PcapScheme
			}

			snaplen := mtu + 32
			sw := libtethux_br.NewSwitch(libtethux_br.SwitchOptions{})

			libtethux_br.CleanupLink(hostA)
			libtethux_br.CleanupLink(hostB)

			defer func() {
				if stopErr := sw.Stop(); stopErr != nil {
					log.Printf("switch shutdown error: %v", stopErr)
				}
				libtethux_br.CleanupLink(hostA)
				libtethux_br.CleanupLink(hostB)
			}()

			fmt.Printf("Starting namespace bridge PID %d <-> PID %d\n", pidA, pidB)

			if attachErr := libtethux_br.AttachVethToNamespace(pidA, hostA, containerIf, mtu); attachErr != nil {
				return fmt.Errorf("connect %s to pid %d: %w", hostA, pidA, attachErr)
			}

			if attachBErr := libtethux_br.AttachVethToNamespace(pidB, hostB, containerIf, mtu); attachBErr != nil {
				return fmt.Errorf("connect %s to pid %d: %w", hostB, pidB, attachBErr)
			}

			ports := []portSpec{
				{
					ID:            hostA,
					Scheme:        scheme,
					Interface:     hostA,
					MTU:           mtu,
					ImmediateMode: immediate,
					SnapLen:       snaplen,
				},
				{
					ID:            hostB,
					Scheme:        scheme,
					Interface:     hostB,
					MTU:           mtu,
					ImmediateMode: immediate,
					SnapLen:       snaplen,
				},
			}

			attached, err := attachSwitchPorts(sw, ports)
			if err != nil {
				closePorts(attached)
				return err
			}

			printPortSummary("Namespace bridge", ports)

			return runSwitch(sw)
		},
	}

	cmd.Flags().BoolVar(&usePcap, "pcap", false, "use pcap instead of raw sockets")
	cmd.Flags().BoolVar(&immediate, "immediate", true, "enable pcap immediate mode")
	cmd.Flags().StringVar(&hostA, "host-a", "vethA-host", "host veth name for pid-a")
	cmd.Flags().StringVar(&hostB, "host-b", "vethB-host", "host veth name for pid-b")
	cmd.Flags().StringVar(&containerIf, "container-if", "tx0", "interface name inside each namespace")
	cmd.Flags().IntVar(&mtu, "mtu", 1500, "link MTU")

	return cmd
}

func newBridgeUDPCmd() *cobra.Command {
	var (
		specs []string
		mtu   int
	)

	cmd := &cobra.Command{
		Use:   "udp",
		Short: "Run a usermode switch with repeated UDP port specs",
		Long: "Each --port value uses id:listen:remote.\n" +
			"Example:\n" +
			"  --port left:127.0.0.1:10001:127.0.0.1:11001",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(specs) < 2 {
				return fmt.Errorf("need at least two --port values")
			}

			ports, err := parseUDPShorthandSpecs(specs, mtu)
			if err != nil {
				return err
			}

			sw := libtethux_br.NewSwitch(libtethux_br.SwitchOptions{})
			attached, err := attachSwitchPorts(sw, ports)
			if err != nil {
				closePorts(attached)
				return err
			}

			printPortSummary("Usermode bridge", ports)

			return runSwitch(sw)
		},
	}

	cmd.Flags().StringArrayVar(&specs, "port", []string{
		"left:127.0.0.1:10001:127.0.0.1:11001",
		"right:127.0.0.1:10002:127.0.0.1:11002",
	}, "udp port spec: id:listen:remote")
	cmd.Flags().IntVar(&mtu, "mtu", 1500, "logical port MTU")

	return cmd
}

func parsePortSpecs(specs []string) ([]portSpec, error) {
	ports := make([]portSpec, 0, len(specs))
	seen := make(map[string]struct{}, len(specs))

	for _, raw := range specs {
		spec, err := parsePortSpec(raw)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[spec.ID]; ok {
			return nil, fmt.Errorf("duplicate port id: %s", spec.ID)
		}
		seen[spec.ID] = struct{}{}
		ports = append(ports, spec)
	}

	return ports, nil
}

func parsePortSpec(raw string) (portSpec, error) {
	values := map[string]string{}
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return portSpec{}, fmt.Errorf("invalid port spec %q: want key=value entries", raw)
		}
		values[strings.TrimSpace(strings.ToLower(key))] = strings.TrimSpace(value)
	}

	id := values["id"]
	if id == "" {
		return portSpec{}, fmt.Errorf("invalid port spec %q: missing id", raw)
	}

	scheme := libtethux_br.AvailableScheme(values["scheme"])
	if scheme == "" {
		return portSpec{}, fmt.Errorf("invalid port spec %q: missing scheme", raw)
	}

	spec := portSpec{
		ID:            id,
		Scheme:        scheme,
		MTU:           1500,
		ImmediateMode: true,
	}

	if value := values["if"]; value != "" {
		spec.Interface = value
	}
	if value := values["interface"]; value != "" {
		spec.Interface = value
	}
	if value := values["listen"]; value != "" {
		spec.Listen = value
	}
	if value := values["local"]; value != "" {
		spec.Listen = value
	}
	if value := values["remote"]; value != "" {
		spec.Remote = value
	}
	if value := values["mtu"]; value != "" {
		mtu, err := strconv.Atoi(value)
		if err != nil {
			return portSpec{}, fmt.Errorf("invalid mtu in %q: %w", raw, err)
		}
		spec.MTU = mtu
	}
	if value := values["snaplen"]; value != "" {
		snaplen, err := strconv.Atoi(value)
		if err != nil {
			return portSpec{}, fmt.Errorf("invalid snaplen in %q: %w", raw, err)
		}
		spec.SnapLen = snaplen
	}
	if value := values["immediate"]; value != "" {
		immediate, err := strconv.ParseBool(value)
		if err != nil {
			return portSpec{}, fmt.Errorf("invalid immediate in %q: %w", raw, err)
		}
		spec.ImmediateMode = immediate
	}
	if value := values["latency"]; value != "" {
		latency, err := time.ParseDuration(value)
		if err != nil {
			return portSpec{}, fmt.Errorf("invalid latency in %q: %w", raw, err)
		}
		spec.Latency = latency
	}
	if spec.SnapLen == 0 {
		spec.SnapLen = spec.MTU + 32
	}

	switch spec.Scheme {
	case libtethux_br.UDPScheme:
		if spec.Listen == "" || spec.Remote == "" {
			return portSpec{}, fmt.Errorf("udp port %s needs listen and remote", spec.ID)
		}
	case libtethux_br.RawScheme, libtethux_br.PcapScheme, libtethux_br.TAPScheme:
		if spec.Interface == "" {
			return portSpec{}, fmt.Errorf("%s port %s needs if", spec.Scheme, spec.ID)
		}
	default:
		return portSpec{}, fmt.Errorf("unsupported scheme %q for port %s", spec.Scheme, spec.ID)
	}

	return spec, nil
}

func parseUDPShorthandSpecs(specs []string, mtu int) ([]portSpec, error) {
	ports := make([]portSpec, 0, len(specs))
	seen := make(map[string]struct{}, len(specs))

	for _, raw := range specs {
		parts := strings.Split(raw, ":")
		if len(parts) < 5 {
			return nil, fmt.Errorf("invalid udp port spec %q: want id:host:port:host:port", raw)
		}

		id := parts[0]
		listen := strings.Join(parts[1:3], ":")
		remote := strings.Join(parts[3:5], ":")

		if _, ok := seen[id]; ok {
			return nil, fmt.Errorf("duplicate port id: %s", id)
		}
		seen[id] = struct{}{}

		ports = append(ports, portSpec{
			ID:            id,
			Scheme:        libtethux_br.UDPScheme,
			Listen:        listen,
			Remote:        remote,
			MTU:           mtu,
			ImmediateMode: true,
			SnapLen:       mtu + 32,
		})
	}

	return ports, nil
}

func attachSwitchPorts(sw *libtethux_br.Switch, specs []portSpec) ([]libtethux_br.Port, error) {
	attached := make([]libtethux_br.Port, 0, len(specs))
	for _, spec := range specs {
		port, err := libtethux_br.NewPort(spec.Scheme, &libtethux_br.PortOptions{
			ID:            spec.ID,
			Interface:     spec.Interface,
			LocalAddr:     spec.Listen,
			Remote:        spec.Remote,
			MTU:           spec.MTU,
			ImmediateMode: spec.ImmediateMode,
			SnapLen:       spec.SnapLen,
		})
		if err != nil {
			return attached, fmt.Errorf("create port %s: %w", spec.ID, err)
		}
		if spec.Latency > 0 {
			port = libtethux_br.WithLatency(port, spec.Latency)
		}
		if err := sw.AttachPort(port); err != nil {
			_ = port.Close()
			return attached, fmt.Errorf("attach port %s: %w", spec.ID, err)
		}
		attached = append(attached, port)
	}

	return attached, nil
}

func closePorts(ports []libtethux_br.Port) {
	for _, port := range ports {
		_ = port.Close()
	}
}

func printPortSummary(title string, specs []portSpec) {
	fmt.Println(title + ":")

	ordered := append([]portSpec(nil), specs...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].ID < ordered[j].ID
	})

	for _, spec := range ordered {
		switch spec.Scheme {
		case libtethux_br.UDPScheme:
			fmt.Printf("  %s scheme=%s listen=%s remote=%s mtu=%d\n", spec.ID, spec.Scheme, spec.Listen, spec.Remote, spec.MTU)
		case libtethux_br.RawScheme, libtethux_br.PcapScheme, libtethux_br.TAPScheme:
			fmt.Printf("  %s scheme=%s if=%s mtu=%d snaplen=%d immediate=%t latency=%s\n", spec.ID, spec.Scheme, spec.Interface, spec.MTU, spec.SnapLen, spec.ImmediateMode, spec.Latency)
		default:
			fmt.Printf("  %s scheme=%s\n", spec.ID, spec.Scheme)
		}
	}
}

func runSwitch(sw *libtethux_br.Switch) error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigChan)

	if err := sw.Start(); err != nil {
		return fmt.Errorf("start switch: %w", err)
	}
	defer func() {
		if err := sw.Stop(); err != nil {
			log.Printf("switch shutdown error: %v", err)
		}
	}()

	fmt.Println("Switch running. Press Ctrl+C to stop.")
	<-sigChan
	fmt.Println("\nStopping switch...")

	return nil
}
