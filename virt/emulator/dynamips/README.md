# Dynamips provider

`dynamips.Adapter` implements the common workload lifecycle and typed router
creation through the published `github.com/tethux/dynamips-downgrade` bindings.
The native runtime is process-wide and can be initialized once. One runtime
contains multiple independent router VMs. Calls must be externally serialized.

Supported families are `c1700`, `c2600`, `c2691`, `c3600`, `c3725`, `c3745`, and
`c7200`. `Config` selects a local IOS image, RAM, platform, cards, native port
mappings, UDP endpoints, and TCP console port. C7200 NPE, midplane, and MAC
overrides are available; generic chassis and non-C7200 MAC overrides are rejected.

Create an adapter with a caller-owned native runtime and construct a `Manager`
from a storage manager. `Manager.Create` prepares an exclusive writable directory
and creates a stopped router. Start it through `Adapter.Start`; state, reload,
list, stop, suspend, resume, restart, and delete use the common provider API.
Errors preserve categories and causes in the `errs` package.

`Manager.Delete` deletes the native VM before releasing its storage lease.
NVRAM, flash, disk, and ROMMON files stay in the prepared directory. Nil startup
and private configs reopen saved NVRAM; non-nil values explicitly provision it.
Retain returned resources when creation reports incomplete cleanup or release,
then retry deletion. Close routers before closing the caller-owned runtime.

The enhanced console uses the native library's exported console client. It
provides local editing and history; automation can use the same TCP endpoint.
Linux amd64 archives are bundled in the dependency. No sibling checkout
is required.

See the [CLI guide](../../../cmd/virt/README.md#dynamips-routers),
[topology example](../../../topology/examples/dynamips-pair.toml), and
[real IOS CI suite](../../../tools/ci/README.md#dynamips-integration).
