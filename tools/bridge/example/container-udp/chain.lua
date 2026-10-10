local tx = require("tethux")
local count = tonumber(arg[1]) or 4
local image = arg[2] or "127.0.0.1:5000/tethux/fixture-a:1"
local mtu = tonumber(arg[3]) or 1500
assert(count >= 2 and count <= 254 and count == math.floor(count), "require 2..254 nodes")

local guest_setup = [[
until ip link show eth0 >/dev/null 2>&1; do sleep 0.1; done
interface=eth0
if [ "$BRIDGE" = 1 ]; then
 until ip link show eth1 >/dev/null 2>&1; do sleep 0.1; done
 ip link add br0 type bridge
 ip link set eth0 master br0
 ip link set eth1 master br0
 ip link set br0 up
 interface=br0
fi
ip addr add "$ADDRESS" dev "$interface"
exec sleep infinity
]]

local net = tx.topology("container-udp-test")
local nodes = {}
for i = 1, count do
  local bridge = (i > 1 and i < count) and 1 or 0
  local setup = string.format("ADDRESS=10.77.0.%d/24\nBRIDGE=%d\n", i, bridge) .. guest_setup
  nodes[i] = net:node("node-" .. i, tx.Kind.Container, image, {"sh", "-ec", setup})
  nodes[i]:port("eth0")
  if bridge == 1 then
    nodes[i]:port("eth1")
  end
end

for i = 2, count do
  local left_port = i == 2 and "eth0" or "eth1"
  net:link(nodes[i - 1]:port(left_port), nodes[i]:port("eth0"), mtu)
end

return net
