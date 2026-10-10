package lua

import (
	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
	glua "github.com/yuin/gopher-lua"
)

const (
	builderType = "tethux.topology"
	nodeType    = "tethux.node"
	portType    = "tethux.port"
	errorType   = "tethux.error"
)

type luaError struct {
	err error
}

type userdataValue interface {
	*builder | nodeHandle | portHandle | luaError
}

func register(state *glua.LState) {
	builderMeta := state.NewTypeMetatable(builderType)
	state.SetField(builderMeta, "__index", state.SetFuncs(state.NewTable(), map[string]glua.LGFunction{
		"node": luaNode,
		"link": luaLink,
	}))
	nodeMeta := state.NewTypeMetatable(nodeType)
	state.SetField(nodeMeta, "__index", state.SetFuncs(state.NewTable(), map[string]glua.LGFunction{
		"port": luaPort,
	}))
	state.NewTypeMetatable(portType)
	errorMeta := state.NewTypeMetatable(errorType)
	state.SetField(errorMeta, "__tostring", state.NewFunction(func(state *glua.LState) int {
		failure := checkValue[luaError](state, 1, "topology error")
		state.Push(glua.LString(failure.err.Error()))
		return 1
	}))
	state.PreloadModule("tethux", func(state *glua.LState) int {
		module := state.NewTable()
		state.SetField(module, "topology", state.NewFunction(luaTopology))
		kind := state.NewTable()
		state.SetField(kind, "Container", glua.LString(topology.NodeKindContainer))
		state.SetField(module, "Kind", kind)
		medium := state.NewTable()
		state.SetField(medium, "Ethernet", glua.LString(topology.PortEthernet))
		state.SetField(medium, "Serial", glua.LString(topology.PortSerial))
		state.SetField(module, "Medium", medium)
		state.Push(module)
		return 1
	})
}

func luaTopology(state *glua.LState) int {
	id := topology.ID(state.CheckString(1))
	if id == "" {
		return raiseError(state, errs.New("create Lua topology", errs.ErrInvalidTopology, "id is required"))
	}
	pushValue(state, newBuilder(id), builderType)
	return 1
}

func luaNode(state *glua.LState) int {
	b := checkValue[*builder](state, 1, "topology builder")
	id := topology.NodeID(state.CheckString(2))
	kind := topology.NodeKind(state.CheckString(3))
	image := state.CheckString(4)
	if kind != topology.NodeKindContainer {
		return raiseError(state, errs.New("create Lua node", errs.ErrUnsupported, string(kind)))
	}
	var command []string
	if state.Get(5) != glua.LNil {
		arguments := state.CheckTable(5)
		command = make([]string, arguments.Len())
		arguments.ForEach(func(key, value glua.LValue) {
			index, numeric := key.(glua.LNumber)
			text, valid := value.(glua.LString)
			if !numeric || index < 1 || index > glua.LNumber(len(command)) || index != glua.LNumber(int(index)) || !valid {
				state.ArgError(5, "expected a sequence of command strings")
				return
			}
			command[int(index)-1] = string(text)
		})
		for index := range command {
			if arguments.RawGetInt(index+1) == glua.LNil {
				state.ArgError(5, "expected a sequence of command strings")
			}
		}
	}
	handle, err := b.node(id, topology.ContainerSpec{Image: image, Command: command})
	if err != nil {
		return raiseError(state, err)
	}
	pushValue(state, handle, nodeType)
	return 1
}

func luaPort(state *glua.LState) int {
	node := checkValue[nodeHandle](state, 1, "node handle")
	id := topology.PortID(state.CheckString(2))
	medium := topology.PortKind(state.OptString(3, ""))
	handle, err := node.port(id, medium)
	if err != nil {
		return raiseError(state, err)
	}
	pushValue(state, handle, portType)
	return 1
}

func luaLink(state *glua.LState) int {
	b := checkValue[*builder](state, 1, "topology builder")
	a := checkValue[portHandle](state, 2, "port handle")
	other := checkValue[portHandle](state, 3, "port handle")
	mtu := state.OptNumber(4, 0)
	if mtu < 0 || mtu > 65535 || mtu != glua.LNumber(int(mtu)) {
		return raiseError(state, errs.New("create Lua link", errs.ErrInvalidLink, "MTU must be an integer between 0 and 65535"))
	}
	err := b.link(a, other, uint16(mtu))
	if err != nil {
		return raiseError(state, err)
	}
	return 0
}

func checkValue[T userdataValue](state *glua.LState, index int, name string) T {
	userdata := state.CheckUserData(index)
	value, ok := userdata.Value.(T)
	if !ok {
		state.ArgError(index, "expected "+name)
	}
	return value
}

func pushValue[T userdataValue](state *glua.LState, value T, name string) {
	userdata := state.NewUserData()
	userdata.Value = value
	state.SetMetatable(userdata, state.GetTypeMetatable(name))
	state.Push(userdata)
}

func raiseError(state *glua.LState, err error) int {
	pushValue(state, luaError{err: err}, errorType)
	state.Error(state.Get(-1), 1)
	return 0
}
