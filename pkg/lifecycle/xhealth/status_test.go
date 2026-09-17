package xhealth

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatus_IsHealthy(t *testing.T) {
	tests := []struct {
		status Status
		want   bool
	}{
		{StatusUp, true},
		{StatusDegraded, true},
		{StatusDown, false},
		{Status("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.IsHealthy())
		})
	}
}

func TestCheckResult_MarshalJSON(t *testing.T) {
	cr := CheckResult{
		Status:   StatusUp,
		Duration: 2 * time.Millisecond,
	}

	data, err := json.Marshal(cr)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	assert.Equal(t, "up", m["status"])
	assert.Equal(t, "2ms", m["duration"])
	assert.Nil(t, m["error"]) // omitempty

	// 带错误信息
	cr2 := CheckResult{
		Status:   StatusDown,
		Error:    "connection refused",
		Duration: time.Second,
	}

	data2, err := json.Marshal(cr2)
	require.NoError(t, err)

	var m2 map[string]any
	require.NoError(t, json.Unmarshal(data2, &m2))

	assert.Equal(t, "down", m2["status"])
	assert.Equal(t, "1s", m2["duration"])
	assert.Equal(t, "connection refused", m2["error"])
}

func TestResult_JSON(t *testing.T) {
	r := Result{
		Status: StatusDegraded,
		Checks: map[string]CheckResult{
			"db":    {Status: StatusUp, Duration: time.Millisecond},
			"redis": {Status: StatusDegraded, Error: "slow", Duration: 500 * time.Millisecond},
		},
	}

	data, err := json.Marshal(r)
	require.NoError(t, err)

	var r2 Result
	require.NoError(t, json.Unmarshal(data, &r2))

	assert.Equal(t, StatusDegraded, r2.Status)
	assert.Len(t, r2.Checks, 2)
	assert.Equal(t, StatusUp, r2.Checks["db"].Status)
	assert.Equal(t, "slow", r2.Checks["redis"].Error)
}

func TestStatusCode(t *testing.T) {
	assert.Equal(t, 200, statusCode(StatusUp))
	assert.Equal(t, 200, statusCode(StatusDegraded))
	assert.Equal(t, 503, statusCode(StatusDown))
}

func TestAggregate(t *testing.T) {
	tests := []struct {
		name    string
		results map[string]CheckResult
		entries []*checkEntry
		want    Status
	}{
		{
			name: "全部通过",
			entries: []*checkEntry{
				{name: "a"},
				{name: "b"},
			},
			results: map[string]CheckResult{
				"a": {Status: StatusUp},
				"b": {Status: StatusUp},
			},
			want: StatusUp,
		},
		{
			name: "有降级",
			entries: []*checkEntry{
				{name: "a"},
				{name: "b"},
			},
			results: map[string]CheckResult{
				"a": {Status: StatusUp},
				"b": {Status: StatusDegraded},
			},
			want: StatusDegraded,
		},
		{
			name: "有故障",
			entries: []*checkEntry{
				{name: "a"},
				{name: "b"},
			},
			results: map[string]CheckResult{
				"a": {Status: StatusDegraded},
				"b": {Status: StatusDown},
			},
			want: StatusDown,
		},
		{
			name:    "无检查项",
			entries: nil,
			results: map[string]CheckResult{},
			want:    StatusUp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := aggregate(tt.entries, tt.results)
			assert.Equal(t, tt.want, r.Status)
		})
	}
}
