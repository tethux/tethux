package main

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestExpensiveAssertionsRequireEnabledGuard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "unguarded",
			src:  `package p; func f() { s.assertValidLocked() }`,
			want: 1,
		},
		{
			name: "guarded",
			src:  `package p; func f() { if assert.Enabled { s.assertValidLocked() } }`,
		},
		{
			name: "guarded conjunction",
			src:  `package p; func f() { if ready && assert.Enabled { s.assertValidLocked() } }`,
		},
		{
			name: "guarded conjunction reversed",
			src:  `package p; func f() { if assert.Enabled && ready { s.assertValidLocked() } }`,
		},
		{
			name: "unsafe disjunction",
			src:  `package p; func f() { if ready || assert.Enabled { s.assertValidLocked() } }`,
			want: 1,
		},
		{
			name: "nested in guard",
			src:  `package p; func f() { if assert.Enabled { for range xs { s.assertInvariants() } } }`,
		},
		{
			name: "nested condition retains guard",
			src: `package p
func f() {
	if assert.Enabled {
		if ready {
			s.assertValidLocked()
		}
	}
}`,
		},
		{
			name: "else is not guarded",
			src:  `package p; func f() { if assert.Enabled {} else { s.assertValidLocked() } }`,
			want: 1,
		},
		{
			name: "cheap assertion",
			src:  `package p; func f() { assert.That(x != nil, "nil") }`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fset := token.NewFileSet()
			file, err := parser.ParseFile(
				fset,
				"test.go",
				test.src,
				parser.SkipObjectResolution,
			)
			if err != nil {
				t.Fatal(err)
			}

			var diagnostics []diagnostic

			lintAssertions(
				fset,
				file,
				false,
				func(d diagnostic) {
					diagnostics = append(diagnostics, d)
				},
			)

			if got := len(diagnostics); got != test.want {
				t.Fatalf(
					"diagnostics = %d, want %d: %+v",
					got,
					test.want,
					diagnostics,
				)
			}

			for _, d := range diagnostics {
				if d.Rule != ruleExpensiveAssertion {
					t.Errorf(
						"rule = %q, want %q",
						d.Rule,
						ruleExpensiveAssertion,
					)
				}

				if d.Message == "" {
					t.Error("diagnostic has empty message")
				}

				if d.Suggestion == "" {
					t.Error("diagnostic has empty suggestion")
				}
			}
		})
	}
}

func TestStructuredErrorsScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{
			path: "storage/local/local.go",
			want: true,
		},
		{
			path: "storage/ref.go",
			want: true,
		},
		{
			path: "bridge/bridge.go",
			want: true,
		},
		{
			path: "bridge/backend/backend.go",
			want: true,
		},
		{
			path: "storage/errs/errors.go",
			want: false,
		},
		{
			path: "bridge/errs/errors.go",
			want: false,
		},
		{
			path: "storage/local/local_test.go",
			want: false,
		},
		{
			path: "bridge/bridge_test.go",
			want: false,
		},
		{
			path: "virt/vm.go",
			want: false,
		},
		{
			path: "internal/foo.go",
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()

			got := enforcesStructuredErrors(test.path)
			if got != test.want {
				t.Fatalf(
					"enforcesStructuredErrors(%q) = %v, want %v",
					test.path,
					got,
					test.want,
				)
			}
		})
	}
}

func TestStructuredErrorsRejectGenericConstructors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		src         string
		want        int
		wantMessage string
	}{
		{
			name: "fmt Errorf",
			src: `
package p

import "fmt"

func f() error {
	return fmt.Errorf("bad: %s", value)
}
`,
			want:        1,
			wantMessage: "fmt.Errorf",
		},
		{
			name: "errors New",
			src: `
package p

import "errors"

func f() error {
	return errors.New("bad")
}
`,
			want:        1,
			wantMessage: "errors.New",
		},
		{
			name: "aliased fmt",
			src: `
package p

import f "fmt"

func bad() error {
	return f.Errorf("bad")
}
`,
			want:        1,
			wantMessage: "fmt.Errorf",
		},
		{
			name: "aliased errors",
			src: `
package p

import e "errors"

func bad() error {
	return e.New("bad")
}
`,
			want:        1,
			wantMessage: "errors.New",
		},
		{
			name: "structured New",
			src: `
package p

import errs "example.com/project/storage/errs"

func f() error {
	return errs.New("op", errs.ErrBad, "target")
}
`,
		},
		{
			name: "structured Wrap",
			src: `
package p

import errs "example.com/project/storage/errs"

func f(cause error) error {
	return errs.Wrap("op", errs.ErrBad, "target", cause)
}
`,
		},
		{
			name: "unrelated Errorf method",
			src: `
package p

func f(logger Logger) error {
	return logger.Errorf("bad")
}
`,
		},
		{
			name: "local fmt variable",
			src: `
package p

func f(fmt Formatter) error {
	return fmt.Errorf("bad")
}
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fset := token.NewFileSet()
			file, err := parser.ParseFile(
				fset,
				"test.go",
				test.src,
				parser.SkipObjectResolution,
			)
			if err != nil {
				t.Fatal(err)
			}

			var diagnostics []diagnostic

			lintErrorConstructors(
				fset,
				file,
				func(d diagnostic) {
					diagnostics = append(diagnostics, d)
				},
			)

			if got := len(diagnostics); got != test.want {
				t.Fatalf(
					"diagnostics = %d, want %d: %+v",
					got,
					test.want,
					diagnostics,
				)
			}

			for _, d := range diagnostics {
				if d.Rule != ruleStructuredError {
					t.Errorf(
						"rule = %q, want %q",
						d.Rule,
						ruleStructuredError,
					)
				}

				if d.Suggestion == "" {
					t.Error("diagnostic has empty suggestion")
				}
			}

			if test.wantMessage != "" {
				if len(diagnostics) == 0 {
					t.Fatalf(
						"expected diagnostic containing %q",
						test.wantMessage,
					)
				}

				if !strings.Contains(
					diagnostics[0].Message,
					test.wantMessage,
				) {
					t.Fatalf(
						"message = %q, want substring %q",
						diagnostics[0].Message,
						test.wantMessage,
					)
				}
			}
		})
	}
}

func TestImportAliases(t *testing.T) {
	t.Parallel()

	const src = `
package p

import (
	"fmt"
	e "errors"
	_ "net/http/pprof"
	. "strings"
)
`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(
		fset,
		"test.go",
		src,
		parser.SkipObjectResolution,
	)
	if err != nil {
		t.Fatal(err)
	}

	got := importAliases(file)

	want := map[string]string{
		"fmt": "fmt",
		"e":   "errors",
	}

	if len(got) != len(want) {
		t.Fatalf("imports = %#v, want %#v", got, want)
	}

	for alias, importPath := range want {
		if got[alias] != importPath {
			t.Errorf(
				"imports[%q] = %q, want %q",
				alias,
				got[alias],
				importPath,
			)
		}
	}
}

func TestParseIgnoreDirective(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		comment string
		want    string
		ok      bool
	}{
		{
			name:    "assertion",
			comment: "//repolint:ignore expensive-assertion",
			want:    "expensive-assertion",
			ok:      true,
		},
		{
			name:    "structured",
			comment: "// repolint:ignore structured-error -- intentional boundary",
			want:    "structured-error",
			ok:      true,
		},
		{
			name:    "all",
			comment: "//repolint:ignore all -- generated compatibility code",
			want:    "all",
			ok:      true,
		},
		{
			name:    "not directive",
			comment: "// something else",
		},
		{
			name:    "missing rule",
			comment: "//repolint:ignore",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, ok := parseIgnoreDirective(test.comment)

			if ok != test.ok {
				t.Fatalf(
					"ok = %v, want %v",
					ok,
					test.ok,
				)
			}

			if got != test.want {
				t.Fatalf(
					"rule = %q, want %q",
					got,
					test.want,
				)
			}
		})
	}
}

func TestIgnoreDirective(t *testing.T) {
	t.Parallel()

	const src = `package p

import "fmt"

func f() error {
	//repolint:ignore structured-error -- compatibility boundary
	return fmt.Errorf("bad")
}
`

	fset := token.NewFileSet()

	file, err := parser.ParseFile(
		fset,
		"storage/foo.go",
		src,
		parser.ParseComments|parser.SkipObjectResolution,
	)
	if err != nil {
		t.Fatal(err)
	}

	var diagnostics []diagnostic

	lintErrorConstructors(
		fset,
		file,
		func(d diagnostic) {
			diagnostics = append(diagnostics, d)
		},
	)

	if len(diagnostics) != 0 {
		t.Fatalf(
			"diagnostics = %+v, want none",
			diagnostics,
		)
	}
}

func TestIgnoreDirectiveDoesNotSuppressDifferentRule(t *testing.T) {
	t.Parallel()

	const src = `package p

import "fmt"

func f() error {
	//repolint:ignore expensive-assertion -- unrelated
	return fmt.Errorf("bad")
}
`

	fset := token.NewFileSet()

	file, err := parser.ParseFile(
		fset,
		"storage/foo.go",
		src,
		parser.ParseComments|parser.SkipObjectResolution,
	)
	if err != nil {
		t.Fatal(err)
	}

	var diagnostics []diagnostic

	lintErrorConstructors(
		fset,
		file,
		func(d diagnostic) {
			diagnostics = append(diagnostics, d)
		},
	)

	if len(diagnostics) != 1 {
		t.Fatalf(
			"diagnostics = %d, want 1: %+v",
			len(diagnostics),
			diagnostics,
		)
	}
}

func TestErrorfWrapSuggestion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		wrap bool
	}{
		{
			name: "wraps cause",
			src: `
package p
import "fmt"
func f(err error) error {
	return fmt.Errorf("open: %w", err)
}`,
			wrap: true,
		},
		{
			name: "does not wrap",
			src: `
package p
import "fmt"
func f() error {
	return fmt.Errorf("bad value: %s", value)
}`,
		},
		{
			name: "escaped percent",
			src: `
package p
import "fmt"
func f() error {
	return fmt.Errorf("literal %%w")
}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fset := token.NewFileSet()

			file, err := parser.ParseFile(
				fset,
				"test.go",
				test.src,
				parser.SkipObjectResolution,
			)
			if err != nil {
				t.Fatal(err)
			}

			var call *ast.CallExpr

			ast.Inspect(file, func(node ast.Node) bool {
				candidate, ok := node.(*ast.CallExpr)
				if ok {
					call = candidate
					return false
				}

				return true
			})

			if call == nil {
				t.Fatal("fmt.Errorf call not found")
			}

			if got := errorfWrapsCause(call); got != test.wrap {
				t.Fatalf(
					"errorfWrapsCause = %v, want %v",
					got,
					test.wrap,
				)
			}
		})
	}
}

func TestContainsErrorWrapVerb(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format string
		want   bool
	}{
		{
			format: "open: %w",
			want:   true,
		},
		{
			format: "%s: %w",
			want:   true,
		},
		{
			format: "literal %%w",
			want:   false,
		},
		{
			format: "value: %v",
			want:   false,
		},
		{
			format: "plain text",
			want:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.format, func(t *testing.T) {
			t.Parallel()

			if got := containsErrorWrapVerb(test.format); got != test.want {
				t.Fatalf(
					"containsErrorWrapVerb(%q) = %v, want %v",
					test.format,
					got,
					test.want,
				)
			}
		})
	}
}

func TestFixAssertions(t *testing.T) {
	t.Parallel()

	const src = `package p

func f() {
	s.assertValidLocked()

	for range xs {
		s.assertInvariants()
	}

	if ready {
		s.assertValidLocked()
	}

	if assert.Enabled {
		s.assertValidLocked()
	}
}
`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(
		fset,
		"test.go",
		src,
		parser.ParseComments|parser.SkipObjectResolution,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !fixAssertions(file, fset) {
		t.Fatal("fixAssertions reported no changes")
	}

	var out bytes.Buffer
	if err := format.Node(&out, fset, file); err != nil {
		t.Fatal(err)
	}

	got := out.String()

	if count := strings.Count(got, "if assert.Enabled"); count != 4 {
		t.Fatalf(
			"assert.Enabled guards = %d, want 4\n%s",
			count,
			got,
		)
	}

	// Running the fixer twice must be idempotent.
	if fixAssertions(file, fset) {
		t.Fatalf(
			"second fix changed already-fixed source:\n%s",
			got,
		)
	}
}
