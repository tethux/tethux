package main

import (
	"bytes"
	"regexp"
	"testing"
)

func TestTOMLOutputPreservesInput(t *testing.T) {
	input, err := chainTOML(config{n: 2, image: "alpine", mtu: 1500})
	if err != nil {
		t.Fatal(err)
	}
	var colored, plain bytes.Buffer
	err = printTOML(&colored, input, true)
	if err != nil {
		t.Fatal(err)
	}
	err = printTOML(&plain, input, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(colored.Bytes(), []byte("\x1b[")) {
		t.Fatal("colored output has no ANSI colors")
	}
	stripped := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAll(colored.Bytes(), nil)
	if !bytes.Equal(stripped, plain.Bytes()) {
		t.Fatal("syntax colors changed the TOML")
	}
	if !bytes.Contains(plain.Bytes(), input) || bytes.Contains(plain.Bytes(), []byte("\x1b[")) {
		t.Fatal("plain output does not preserve the document")
	}
}
