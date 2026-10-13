// Package local runs container, domain, Dynamips, and Shitnet topologies on one Linux host using UDP links.
//
// SelectContainerProvider selects a reachable Docker, Podman, or containerd
// provider, or a caller can supply an existing container.ContainerProvider in Options. Start
// validates the complete topology and its supported resource types before
// creating network-isolated containers. Missing images are pulled through the
// selected provider. Each declared Ethernet link creates two namespace veth
// attachments connected by a pair of loopback UDP bridges.
//
// Linked port IDs become Linux interface names. Guest commands own address and
// routing configuration; Start prepares workloads and links. DynamipsOptions
// supplies a native adapter and persistent state root for router workloads.
// DomainOptions supplies a hypervisor, qcow2 base image, and optional seed ISO.
// Shitnet endpoints supply native IPv4 stacks on UDP Ethernet links.
// Views have no effect on execution. Serial links are unsupported.
//
// Running owns its resources, and a failed Start rolls them back. The caller
// must close a successful run with a live cleanup context. Close is idempotent
// and retains failed container deletions for retry. It only removes resources
// belonging to that run. Running methods are not safe for concurrent use.
//
// Options.Logger optionally receives slog lifecycle progress. Nil disables
// logging without changing the process's default logger. The caller retains
// ownership of the provider and logger. Namespace attachment requires root
// privileges on the same Linux host as the container provider.
package local
