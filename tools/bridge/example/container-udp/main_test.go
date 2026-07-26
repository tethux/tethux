package main

import "testing"

func TestBridgeCommandArgsStartWithContainer(t *testing.T) {
	args := bridgeCommandArgs(config{mtu: 1500}, node{pid: "123", hostIf: "txhost", containerIf: "tx01"})
	if len(args) == 0 || args[0] != "container" {
		t.Fatalf("bridge-specific binary must receive container as its subcommand, got %#v", args)
	}
}
