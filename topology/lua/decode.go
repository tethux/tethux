package lua

import (
	"errors"
	"io"

	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
	glua "github.com/yuin/gopher-lua"
)

// Decode executes a Lua script and returns its validated topology plan.
// Scripts must return a builder created by require("tethux").topology.
// The caller retains ownership of the reader. Scripts are trusted code and
// have access to the standard Lua libraries; Decode does not start workloads.
// Optional script arguments are exposed as the Lua arg sequence, starting at 1.
func Decode(reader io.Reader, args ...string) (*topology.Topology, error) {
	if reader == nil {
		return nil, errs.New("decode topology Lua", errs.ErrDecode, "nil reader")
	}
	state := glua.NewState()
	defer state.Close()
	register(state)
	arguments := state.NewTable()
	for _, argument := range args {
		arguments.Append(glua.LString(argument))
	}
	state.SetGlobal("arg", arguments)
	chunk, err := state.Load(reader, "topology")
	if err != nil {
		return nil, decodeError(err)
	}
	err = state.CallByParam(glua.P{Fn: chunk, NRet: 1, Protect: true})
	if err != nil {
		return nil, decodeError(err)
	}
	userdata, ok := state.Get(-1).(*glua.LUserData)
	if !ok {
		return nil, errs.New("decode topology Lua", errs.ErrDecode, "script must return a topology builder")
	}
	b, valid := userdata.Value.(*builder)
	if !valid || b == nil {
		return nil, errs.New("decode topology Lua", errs.ErrDecode, "script must return a topology builder")
	}
	err = b.top.Validate()
	if err != nil {
		return nil, decodeError(err)
	}
	return &b.top, nil
}

func decodeError(err error) error {
	var apiError *glua.ApiError
	if errors.As(err, &apiError) {
		if userdata, ok := apiError.Object.(*glua.LUserData); ok {
			if failure, valid := userdata.Value.(luaError); valid {
				err = errors.Join(err, failure.err)
			}
		}
		if apiError.Cause != nil {
			err = errors.Join(err, apiError.Cause)
		}
	}
	return errs.Wrap("decode topology Lua", errs.ErrDecode, "", err)
}
