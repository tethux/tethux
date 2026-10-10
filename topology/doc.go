// Package topology describes provider- and host-independent network topologies.
//
// A Topology owns nodes, point-to-point links, and optional views. Node IDs,
// link IDs, and view IDs are unique within their own collections; port IDs are
// local to a node. Links reference ports through Endpoint values, and each
// port can participate in at most one link. Views may place any subset of the
// nodes and links without changing connectivity.
//
// Node specs are concrete values from the closed NodeSpec family. Validate
// checks the declarative model without accessing images, allocating resources,
// or selecting a provider. The topology/toml package decodes and validates
// the same model from TOML documents; topology/lua builds it from trusted Lua
// scripts. The topology/local package realizes
// container workloads and Ethernet links on one Linux host.
package topology
