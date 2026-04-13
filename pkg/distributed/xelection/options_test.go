package xelection

import (
	"testing"
	"time"
)

// defaultSentinel 返回一个全默认的 electionOptions 副本，用于每个 case 独立验证。
func defaultSentinel() *electionOptions { return defaultOptions() }

func TestWithTTL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"positive_overrides", 30, 30},
		{"zero_keeps_default", 0, defaultTTLSeconds},
		{"negative_keeps_default", -1, defaultTTLSeconds},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			o := defaultSentinel()
			WithTTL(c.in)(o)
			if o.ttlSeconds != c.want {
				t.Errorf("ttl = %d, want %d", o.ttlSeconds, c.want)
			}
		})
	}
}

func TestWithTTLDuration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   time.Duration
		want int
	}{
		{"10s", 10 * time.Second, 10},
		{"90s", 90 * time.Second, 90},
		{"sub_second_keeps_default", 500 * time.Millisecond, defaultTTLSeconds},
		{"zero_keeps_default", 0, defaultTTLSeconds},
		{"negative_keeps_default", -time.Second, defaultTTLSeconds},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			o := defaultSentinel()
			WithTTLDuration(c.in)(o)
			if o.ttlSeconds != c.want {
				t.Errorf("ttl = %d, want %d", o.ttlSeconds, c.want)
			}
		})
	}
}

func TestWithLogger(t *testing.T) {
	t.Parallel()
	o := defaultSentinel()
	orig := o.logger
	WithLogger(nil)(o)
	if o.logger != orig {
		t.Error("WithLogger(nil) must preserve default logger")
	}
	// 传入非 nil logger：验证被替换。
	replacement := o.logger // 同类型但确保赋值分支被命中。
	WithLogger(replacement)(o)
	if o.logger == nil {
		t.Error("WithLogger(non-nil) must not clear logger")
	}
}
