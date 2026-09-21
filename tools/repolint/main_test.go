package main

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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
			path: "virt/hypervisor/libvirt/domain.go",
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
			path: "virt/hypervisor/libvirt/errs/errors.go",
			want: false,
		},
		{
			path: "virt/hypervisor/libvirt/domain_test.go",
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

func TestStructuredErrorFixUsesDiscoveredStorageDialect(t *testing.T) {
	t.Parallel()

	root := newFixtureModule(t)
	writeFixture(t, root, "storage/errs/errors.go", `package errs
import "errors"
var ErrOpen = errors.New("failed to open storage object")
func New(provider string, kind error, target string) error { return kind }
func Wrap(provider string, kind error, target string, cause error) error { return kind }
`)
	path := writeFixture(t, root, "storage/local/open.go", `package local
import "fmt"
func open(ref string, err error) error {
	return fmt.Errorf("failed to open %q: %w", ref, err)
}
`)

	var diagnostics []diagnostic
	if err := lintRoot(root, options{fix: true}, func(d diagnostic) {
		diagnostics = append(diagnostics, d)
	}); err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics after fix = %+v", diagnostics)
	}

	got := readFixture(t, path)
	for _, want := range []string{
		`storageerrs "example.test/repolint/storage/errs"`,
		`storageerrs.Wrap("storage", storageerrs.ErrOpen, ref, err)`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fixed source does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"fmt"`) {
		t.Fatalf("unused fmt import remains:\n%s", got)
	}
}

func TestStructuredErrorFixCreatesAndDeduplicatesSentinel(t *testing.T) {
	t.Parallel()

	root := newFixtureModule(t)
	writeFixture(t, root, "bridge/errs/errors.go", `package errs
import "errors"
// ErrPortSetup documents the existing sentinel.
var ErrPortSetup = errors.New("failed to set up port")
// OpError documents the existing structured error.
type OpError struct{}
func New(operation string, kind error, target string) error { return kind }
func Wrap(operation string, kind error, target string, cause error) error { return kind }
`)
	path := writeFixture(t, root, "bridge/widget.go", `package bridge
import "errors"
func first() error { return errors.New("failed to frobnicate object") }
func second() error { return errors.New("failed to frobnicate object") }
`)

	if err := lintRoot(root, options{fix: true}, func(d diagnostic) {
		t.Errorf("unexpected diagnostic: %+v", d)
	}); err != nil {
		t.Fatal(err)
	}

	got := readFixture(t, path)
	if count := strings.Count(got, "errs.New"); count != 2 {
		t.Fatalf("structured calls = %d, want 2:\n%s", count, got)
	}
	errorsSource := readFixture(t, filepath.Join(root, filepath.FromSlash("bridge/errs/errors.go")))
	if count := strings.Count(errorsSource, "ErrFrobnicate ="); count != 1 {
		t.Fatalf("generated sentinel count = %d, want 1:\n%s", count, errorsSource)
	}
	if !strings.Contains(errorsSource, "// ErrPortSetup documents the existing sentinel.\nvar ErrPortSetup = errors.New") ||
		!strings.Contains(errorsSource, "// OpError documents the existing structured error.\ntype OpError struct") {
		t.Fatalf("existing error domain was changed unexpectedly:\n%s", errorsSource)
	}
}

func TestStructuredErrorFixUsesFixedProviderDialect(t *testing.T) {
	t.Parallel()

	root := newFixtureModule(t)
	writeFixture(t, root, "virt/hypervisor/libvirt/errs/errors.go", `package errs
import "errors"
var ErrInspect = errors.New("failed to inspect libvirt domain")
func New(kind error, target string) error { return kind }
func Wrap(kind error, target string, cause error) error { return kind }
`)
	path := writeFixture(t, root, "virt/hypervisor/libvirt/domain.go", `package libvirt
import "fmt"
func inspect(id string, err error) error {
	return fmt.Errorf("failed to inspect %q: %w", id, err)
}
`)

	if err := lintRoot(root, options{fix: true}, func(d diagnostic) {
		t.Errorf("unexpected diagnostic: %+v", d)
	}); err != nil {
		t.Fatal(err)
	}

	got := readFixture(t, path)
	if !strings.Contains(got, `errs.Wrap(errs.ErrInspect, id, err)`) {
		t.Fatalf("fixed source uses wrong dialect:\n%s", got)
	}
}

func TestStructuredErrorFixAvoidsAliasCollisionAndKeepsTargets(t *testing.T) {
	t.Parallel()

	root := newFixtureModule(t)
	writeFixture(t, root, "bridge/errs/errors.go", `package errs
import "errors"
var ErrConnect = errors.New("failed to connect bridge endpoints")
func New(operation string, kind error, target string) error { return kind }
func Wrap(operation string, kind error, target string, cause error) error { return kind }
`)
	path := writeFixture(t, root, "bridge/connect.go", `package bridge
import "fmt"
var errs = "occupied"
func connect(left string, right int, cause error) error {
	return fmt.Errorf("failed to connect %q to %d: %w", left, right, cause)
}
`)

	if err := lintRoot(root, options{fix: true}, func(d diagnostic) {
		t.Errorf("unexpected diagnostic: %+v", d)
	}); err != nil {
		t.Fatal(err)
	}

	got := readFixture(t, path)
	for _, want := range []string{
		`errs2 "example.test/repolint/bridge/errs"`,
		`errs2.Wrap("connect to", errs2.ErrConnect, fmt.Sprint(left, " ", right), cause)`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fixed source does not contain %q:\n%s", want, got)
		}
	}
}

func TestSentinelIdentifier(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"failed to open storage object": "ErrOpen",
		"failed to frobnicate object":   "ErrFrobnicate",
		"invalid widget configuration":  "ErrInvalidWidgetConfig",
	}
	for message, want := range tests {
		if got := sentinelIdentifier(message); got != want {
			t.Errorf("sentinelIdentifier(%q) = %q, want %q", message, got, want)
		}
	}
}

func TestStructuredErrorFixCreatesDomain(t *testing.T) {
	t.Parallel()

	root := newFixtureModule(t)
	path := writeFixture(t, root, "virt/hypervisor/qemu/domain.go", `package qemu
import "fmt"
func start(id string, err error) error {
	return fmt.Errorf("failed to start object %q: %w", id, err)
}
`)

	if err := lintRoot(root, options{fix: true}, func(d diagnostic) {
		t.Errorf("unexpected diagnostic: %+v", d)
	}); err != nil {
		t.Fatal(err)
	}

	got := readFixture(t, path)
	if !strings.Contains(got, `errs.Wrap(errs.ErrStart, id, err)`) {
		t.Fatalf("fixed source uses wrong generated domain:\n%s", got)
	}
	errorsSource := readFixture(t, filepath.Join(root, filepath.FromSlash("virt/hypervisor/qemu/errs/errors.go")))
	for _, want := range []string{"type OpError struct", "func (e *OpError) Unwrap() []error", "func New(", "func Wrap("} {
		if !strings.Contains(errorsSource, want) {
			t.Errorf("generated errors.go does not contain %q:\n%s", want, errorsSource)
		}
	}
}

func TestDirectStructuredErrorComparison(t *testing.T) {
	t.Parallel()

	const src = `package p
import storageerrs "example.test/storage/errs"
func bad(err error) bool { return err == storageerrs.ErrOpen }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "storage/bad.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics []diagnostic
	lintDirectErrorComparisons(fset, file, func(d diagnostic) {
		diagnostics = append(diagnostics, d)
	})
	if len(diagnostics) != 1 || diagnostics[0].Rule != ruleErrorComparison {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func newFixtureModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.test/repolint\n\ngo 1.26.4\n")
	return root
}

func writeFixture(t *testing.T, root, name, value string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	// #nosec G306 -- fixture Go files use normal source permissions.
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFixture(t *testing.T, path string) string {
	t.Helper()
	// #nosec G304 -- tests only read paths inside their temporary fixture.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
