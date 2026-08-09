package main

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestExpensiveAssertionsRequireEnabledGuard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want int
	}{
		{name: "unguarded", src: `package p; func f() { s.assertValidLocked() }`, want: 1},
		{name: "guarded", src: `package p; func f() { if assert.Enabled { s.assertValidLocked() } }`},
		{name: "guarded conjunction", src: `package p; func f() { if ready && assert.Enabled { s.assertValidLocked() } }`},
		{name: "unsafe disjunction", src: `package p; func f() { if ready || assert.Enabled { s.assertValidLocked() } }`, want: 1},
		{name: "nested in guard", src: `package p; func f() { if assert.Enabled { for range xs { s.assertInvariants() } } }`},
		{name: "else is not guarded", src: `package p; func f() { if assert.Enabled {} else { s.assertValidLocked() } }`, want: 1},
		{name: "cheap assertion", src: `package p; func f() { assert.That(x != nil, "nil") }`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test.go", test.src, 0)
			if err != nil {
				t.Fatal(err)
			}

			got := 0
			lintNode(fset, file, false, func(token.Position, string) { got++ })
			if got != test.want {
				t.Fatalf("diagnostics = %d, want %d", got, test.want)
			}
		})
	}
}

func TestStructuredErrorsRejectGenericConstructors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want int
	}{
		{name: "fmt Errorf", src: `package p; func f() error { return fmt.Errorf("bad: %s", value) }`, want: 1},
		{name: "errors New", src: `package p; func f() error { return errors.New("bad") }`, want: 1},
		{name: "structured New", src: `package p; func f() error { return errs.New("op", errs.ErrBad, "target") }`},
		{name: "structured Wrap", src: `package p; func f() error { return errs.Wrap("op", errs.ErrBad, "target", cause) }`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test.go", test.src, 0)
			if err != nil {
				t.Fatal(err)
			}

			got := 0
			lintErrorConstructors(fset, file, func(token.Position, string) { got++ })
			if got != test.want {
				t.Fatalf("diagnostics = %d, want %d", got, test.want)
			}
		})
	}
}
