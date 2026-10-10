---@meta

---@enum TethuxKind
local Kind = {
	Container = "container",
}

---@enum TethuxMedium
local Medium = {
	Ethernet = "ethernet",
	Serial = "serial",
}

---@class TethuxPortHandle
local PortHandle = {}

---@class TethuxNodeHandle
local NodeHandle = {}

---@param name string
---@param medium? TethuxMedium
---@return TethuxPortHandle
function NodeHandle:port(name, medium) end

---@class TethuxTopologyBuilder
local TopologyBuilder = {}

---@param id string
---@param kind TethuxKind
---@param image string
---@param command? string[]
---@return TethuxNodeHandle
function TopologyBuilder:node(id, kind, image, command) end

---@param a TethuxPortHandle
---@param b TethuxPortHandle
---@param mtu? integer
function TopologyBuilder:link(a, b, mtu) end

local tx = {
	Kind = Kind,
	Medium = Medium,
}

---@param id string
---@return TethuxTopologyBuilder
function tx.topology(id) end

return tx
