package storageopt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHealthCounter(t *testing.T) {
	var h HealthCounter

	assert.Equal(t, int64(0), h.PingCount())
	assert.Equal(t, int64(0), h.PingErrors())

	h.IncPing()
	h.IncPing()
	h.IncPingError()

	assert.Equal(t, int64(2), h.PingCount())
	assert.Equal(t, int64(1), h.PingErrors())
}

func TestSlowQueryCounter(t *testing.T) {
	var s SlowQueryCounter

	assert.Equal(t, int64(0), s.Count())

	s.Inc()
	s.Inc()
	s.Inc()

	assert.Equal(t, int64(3), s.Count())
}

func TestQueryCounter(t *testing.T) {
	var q QueryCounter

	assert.Equal(t, int64(0), q.QueryCount())
	assert.Equal(t, int64(0), q.QueryErrors())

	q.IncQuery()
	q.IncQuery()
	q.IncQuery()
	q.IncQueryError()

	assert.Equal(t, int64(3), q.QueryCount())
	assert.Equal(t, int64(1), q.QueryErrors())
}

func TestMeasureOperation(t *testing.T) {
	start := time.Now()
	time.Sleep(time.Millisecond)
	d := MeasureOperation(start)
	assert.GreaterOrEqual(t, d.Nanoseconds(), int64(0))
}
