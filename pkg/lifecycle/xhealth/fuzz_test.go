package xhealth

import (
	"context"
	"errors"
	"testing"
)

func FuzzAddCheck(f *testing.F) {
	f.Add("db", true, false, true)
	f.Add("", false, true, false)
	f.Add("a/b/c", true, true, false)
	f.Add("check-with-special-chars!@#", false, false, true)

	f.Fuzz(func(t *testing.T, name string, async bool, skipOnErr bool, shouldFail bool) {
		h, err := New()
		if err != nil {
			t.Skip()
		}

		check := func(_ context.Context) error {
			if shouldFail {
				return errors.New("fail")
			}
			return nil
		}

		cfg := CheckConfig{
			Check:     check,
			Async:     async,
			SkipOnErr: skipOnErr,
		}

		// AddReadinessCheck 不应 panic
		_ = h.AddReadinessCheck(name, cfg) //nolint:errcheck // fuzz test
	})
}

func FuzzCheckResult_MarshalJSON(f *testing.F) {
	f.Add("up", "", int64(0))
	f.Add("down", "connection refused", int64(1000000))
	f.Add("degraded", "slow response", int64(500000000))

	f.Fuzz(func(t *testing.T, status string, errMsg string, durationNs int64) {
		cr := CheckResult{
			Status: Status(status),
			Error:  errMsg,
		}

		// MarshalJSON 不应 panic
		_, _ = cr.MarshalJSON() //nolint:errcheck // fuzz test
	})
}
