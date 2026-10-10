// Package local runs container topologies on one Linux host using UDP links.
//
// SelectContainerProvider selects a reachable Docker, Podman, or containerd
// provider, or a caller can supply an existing container.ContainerProvider in Options. Start
// validates the complete topology and its supported resource types before
// creating network-isolated containers. Missing images are pulled through the
// selected provider. Each declared Ethernet link creates two namespace veth
// attachments connected by a pair of loopback UDP bridges.
//
// Linked port IDs become Linux interface names. Guest commands own address and
// routing configuration; Start only prepares the containers and links. Views
// have no effect on execution. Domain, emulator, and serial resources are not
// supported by this runner.
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
