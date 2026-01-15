package xlog

import (
	"errors"
	"log/slog"
	"testing"
	"time"
)

type fuzzStringer string

func (s fuzzStringer) String() string {
	return string(s)
}

func FuzzAttrBuilders(f *testing.F) {
	f.Add("component", "operation", "user", "/path", int64(1), 200)

	f.Fuzz(func(t *testing.T, component, operation, user, path string, count int64, status int) {
		_ = Component(component)
		_ = Operation(operation)
		_ = UserID(user)
		_ = Path(path)
		_ = Count(count)
		_ = StatusCode(status)
		_ = Duration(fuzzStringer(path))

		resolveAttr(t, Lazy("any", func() any { return component }))
		resolveAttr(t, LazyString("str", func() string { return operation }))
		resolveAttr(t, LazyInt("int", func() int64 { return count }))
		resolveAttr(t, LazyDuration("dur", func() time.Duration { return time.Duration(count) }))

		errAttr := LazyError("err", func() error {
			if count%2 == 0 {
				return errors.New("fuzz error")
			}
			return nil
		})
		resolveAttr(t, errAttr)
	})
}

func resolveAttr(t *testing.T, attr slog.Attr) {
	t.Helper()
	_ = attr.Value.Resolve()
}
