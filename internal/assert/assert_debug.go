//go:build debug

package assert

import (
	"fmt"
	"runtime"
)

const Enabled = true

// That panics when cond is false.
//
// Keep arguments side-effect free: in production builds this function is
// compiled as a no-op, but Go still evaluates ordinary function arguments.
func That(cond bool, format string, args ...any) {
	if cond {
		return
	}
	_, file, line, _ := runtime.Caller(1)
	panic(fmt.Sprintf(
		"assertion failed at %s:%d: %s",
		file,
		line,
		fmt.Sprintf(format, args...),
	))
}

// Lazy avoids evaluating an expensive assertion in production.
func Lazy(check func() bool, message func() string) {
	if check() {
		return
	}
	panic("assertion failed: " + message())
}

// NotNil panics when value is nil.
func NotNil[T any](value *T, name string) *T {
	if value == nil {
		panic("assertion failed: " + name + " must not be nil")
	}
	return value
}
