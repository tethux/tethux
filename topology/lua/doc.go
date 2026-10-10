// Package lua builds provider-independent topology plans from trusted Lua scripts.
//
// require("tethux") exposes topology(id), Kind.Container, and Medium.Ethernet
// and Medium.Serial. Builders provide node(id, kind, image, command) and link(a, b, mtu).
// The optional command is a sequence of strings overriding the image command.
// The optional MTU defaults to zero, leaving sizing to the provider.
// Node handles provide port(name, medium), creating or retrieving a logical
// port. An omitted medium accepts an existing port or defaults to Ethernet.
// Each port can participate in at most one link, within its own topology.
//
// Scripts must return their builder. Decode validates the resulting topology
// without starting workloads. Nodes, ports, and links preserve creation order;
// generated link IDs are link-1, link-2, and so on. Errors preserve topology
// categories and Lua execution causes for errors.Is and errors.As.
package lua
