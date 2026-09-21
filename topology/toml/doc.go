// Package toml decodes declarative topology documents without choosing providers
// or hosts. Decode rejects unknown fields and returns only a validated topology.
// The caller retains ownership of the reader.
//
// A document has an id and nodes, links, and optional views arrays of tables.
// Every node has exactly one container, domain, dynamips, or shitnet spec table.
// Container specs contain image, command, and env; environment entries contain
// name and value. Domain specs contain image, cpus, and memory_mb. Dynamips
// specs contain image and ram_mb. Ports contain explicit id and kind fields.
//
// Link endpoints are inline tables with node and port IDs. For example:
//
//	[[links]]
//	id = "lan"
//	kind = "ethernet"
//	mtu = 1500
//	a = { node = "client", port = "eth0" }
//	b = { node = "server", port = "eth0" }
//
// Views contain node placements with position = { x, y } and link placements
// with points = [{ x, y }]. They describe presentation independently of network
// connectivity. Decoding does not pull images or allocate runtime resources.
// Errors preserve parser causes and topology validation categories.
package toml
