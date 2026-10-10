local tx = require("tethux")
local net = tx.topology("pair")
local a = net:node("a", tx.Kind.Container, "alpine", {"sleep", "infinity"})
local b = net:node("b", tx.Kind.Container, "alpine", {"sleep", "infinity"})
net:link(a:port("eth0"), b:port("eth0"))
return net
