local tx = require("tethux")
local net = tx.topology("ring-6")
local routers = {}

for i = 1, 6 do
  routers[i] = net:node("r" .. i, tx.Kind.Container, "alpine:latest", {"sleep", "infinity"})
end

for i = 1, #routers do
  local next_router = routers[(i % #routers) + 1]
  net:link(
    routers[i]:port("eth1", tx.Medium.Ethernet),
    next_router:port("eth0")
  )
end

return net
