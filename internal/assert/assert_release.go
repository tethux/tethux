//go:build !debug

package assert

const Enabled = false

func That(bool, string, ...any) {}

func Lazy(func() bool, func() string) {}

func NotNil[T any](value *T, _ string) *T {
	return value
}
