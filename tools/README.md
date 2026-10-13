# Repository tools

`tools` contains repository-maintenance programs. These are not public Tethux
library APIs or end-user command packages.

| Path | Purpose | Documentation |
| --- | --- | --- |
| `ci/` | Unified test, archive, remote-runner, and host-management command | [`ci/README.md`](ci/README.md) |
| `bridge/` | Private bridge integration drivers invoked by `tools/ci` | [`bridge/README.md`](bridge/README.md) |
| `libvirt/` | Three-guest Alpine forwarding and lifecycle integration | [`ci/README.md`](ci/README.md#workflows) |
| `topology/` | All-provider Ethernet, native ICMP, lifecycle, and CLI integration | [`ci/README.md`](ci/README.md#all-provider-integration) |
| `dynamips/` | Real IOS topology, lifecycle, persistence, and CLI integration | [`ci/README.md`](ci/README.md#dynamips-integration) |

`tools/ci` owns the monorepo task declarations and local execution. Dagger uses
the same command inside its traced CI functions. Privileged and remote
workflows still go through this CLI; do not invoke implementation drivers directly.

`fixture-registry.sh` manages the local OCI test fixture registry. Alpine test
images come from the pinned Nix integration shell through `TETHUX_LIBVIRT_IMAGE`;
manual runs may supply an existing qcow2 image.
