package xpulsar

import (
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// NewClient 测试
// =============================================================================

func TestNewClient_EmptyURL(t *testing.T) {
	client, err := NewClient("")

	assert.Nil(t, client)
	assert.ErrorIs(t, err, ErrEmptyURL)
}

func TestClientOptions_Default(t *testing.T) {
	opts := defaultOptions()

	assert.NotNil(t, opts.Tracer)
	assert.NotNil(t, opts.Observer)
	assert.Equal(t, 10*time.Second, opts.ConnectionTimeout)
	assert.Equal(t, 30*time.Second, opts.OperationTimeout)
	assert.Equal(t, 1, opts.MaxConnectionsPerBroker)
	assert.Equal(t, 5*time.Second, opts.HealthTimeout)
}

func TestClientOptions_WithTracer(t *testing.T) {
	tracer := NoopTracer{}
	opts := defaultOptions()

	WithTracer(tracer)(opts)

	assert.Equal(t, tracer, opts.Tracer)
}

func TestClientOptions_WithNilTracer(t *testing.T) {
	opts := defaultOptions()
	original := opts.Tracer

	WithTracer(nil)(opts)

	assert.Equal(t, original, opts.Tracer)
}

func TestClientOptions_WithObserver(t *testing.T) {
	opts := defaultOptions()
	observer := xmetrics.NoopObserver{}

	WithObserver(observer)(opts)

	assert.Equal(t, observer, opts.Observer)
}

func TestClientOptions_WithNilObserver(t *testing.T) {
	opts := defaultOptions()
	original := opts.Observer

	WithObserver(nil)(opts)

	assert.Equal(t, original, opts.Observer)
}

func TestClientOptions_WithConnectionTimeout(t *testing.T) {
	opts := defaultOptions()

	WithConnectionTimeout(20 * time.Second)(opts)

	assert.Equal(t, 20*time.Second, opts.ConnectionTimeout)
}

func TestClientOptions_WithConnectionTimeout_Zero(t *testing.T) {
	opts := defaultOptions()
	original := opts.ConnectionTimeout

	WithConnectionTimeout(0)(opts)

	assert.Equal(t, original, opts.ConnectionTimeout)
}

func TestClientOptions_WithOperationTimeout(t *testing.T) {
	opts := defaultOptions()

	WithOperationTimeout(60 * time.Second)(opts)

	assert.Equal(t, 60*time.Second, opts.OperationTimeout)
}

func TestClientOptions_WithMaxConnectionsPerBroker(t *testing.T) {
	opts := defaultOptions()

	WithMaxConnectionsPerBroker(5)(opts)

	assert.Equal(t, 5, opts.MaxConnectionsPerBroker)
}

func TestClientOptions_WithMaxConnectionsPerBroker_Zero(t *testing.T) {
	opts := defaultOptions()
	original := opts.MaxConnectionsPerBroker

	WithMaxConnectionsPerBroker(0)(opts)

	assert.Equal(t, original, opts.MaxConnectionsPerBroker)
}

func TestClientOptions_WithAuthentication(t *testing.T) {
	opts := defaultOptions()

	// 使用 nil 来测试设置
	WithAuthentication(nil)(opts)

	assert.Nil(t, opts.Authentication)
}

func TestClientOptions_WithTLS(t *testing.T) {
	opts := defaultOptions()

	WithTLS("/path/to/cert.pem", true)(opts)

	assert.Equal(t, "/path/to/cert.pem", opts.TLSTrustCertsFilePath)
	assert.True(t, opts.TLSAllowInsecureConnection)
}

func TestClientOptions_WithHealthTimeout(t *testing.T) {
	opts := defaultOptions()

	WithHealthTimeout(10 * time.Second)(opts)

	assert.Equal(t, 10*time.Second, opts.HealthTimeout)
}

// =============================================================================
// Stats 测试
// =============================================================================

func TestStats_ZeroValues(t *testing.T) {
	stats := Stats{}

	assert.False(t, stats.Connected)
	assert.Equal(t, 0, stats.ProducersCount)
	assert.Equal(t, 0, stats.ConsumersCount)
}

func TestStats_WithValues(t *testing.T) {
	stats := Stats{
		Connected:      true,
		ProducersCount: 5,
		ConsumersCount: 3,
	}

	assert.True(t, stats.Connected)
	assert.Equal(t, 5, stats.ProducersCount)
	assert.Equal(t, 3, stats.ConsumersCount)
}

// =============================================================================
// Additional Option Tests
// =============================================================================

func TestClientOptions_WithOperationTimeout_Zero(t *testing.T) {
	opts := defaultOptions()
	original := opts.OperationTimeout

	WithOperationTimeout(0)(opts)

	assert.Equal(t, original, opts.OperationTimeout)
}

func TestClientOptions_WithOperationTimeout_Negative(t *testing.T) {
	opts := defaultOptions()
	original := opts.OperationTimeout

	WithOperationTimeout(-1 * time.Second)(opts)

	assert.Equal(t, original, opts.OperationTimeout)
}

func TestClientOptions_WithConnectionTimeout_Negative(t *testing.T) {
	opts := defaultOptions()
	original := opts.ConnectionTimeout

	WithConnectionTimeout(-1 * time.Second)(opts)

	assert.Equal(t, original, opts.ConnectionTimeout)
}

func TestClientOptions_WithHealthTimeout_Zero(t *testing.T) {
	opts := defaultOptions()
	original := opts.HealthTimeout

	WithHealthTimeout(0)(opts)

	assert.Equal(t, original, opts.HealthTimeout)
}

func TestClientOptions_WithHealthTimeout_Negative(t *testing.T) {
	opts := defaultOptions()
	original := opts.HealthTimeout

	WithHealthTimeout(-1 * time.Second)(opts)

	assert.Equal(t, original, opts.HealthTimeout)
}

func TestClientOptions_WithMaxConnectionsPerBroker_Negative(t *testing.T) {
	opts := defaultOptions()
	original := opts.MaxConnectionsPerBroker

	WithMaxConnectionsPerBroker(-1)(opts)

	assert.Equal(t, original, opts.MaxConnectionsPerBroker)
}

func TestClientOptions_WithTLS_EmptyPath(t *testing.T) {
	opts := defaultOptions()

	WithTLS("", false)(opts)

	assert.Equal(t, "", opts.TLSTrustCertsFilePath)
	assert.False(t, opts.TLSAllowInsecureConnection)
}

func TestClientOptions_WithTLS_InsecureOnly(t *testing.T) {
	opts := defaultOptions()

	WithTLS("", true)(opts)

	assert.Equal(t, "", opts.TLSTrustCertsFilePath)
	assert.True(t, opts.TLSAllowInsecureConnection)
}

// =============================================================================
// WithHealthCheckTopic Tests
// =============================================================================

func TestClientOptions_WithHealthCheckTopic(t *testing.T) {
	opts := defaultOptions()

	WithHealthCheckTopic("non-persistent://my-tenant/my-ns/__health__")(opts)

	assert.Equal(t, "non-persistent://my-tenant/my-ns/__health__", opts.HealthCheckTopic)
}

func TestClientOptions_WithHealthCheckTopic_Empty(t *testing.T) {
	opts := defaultOptions()
	original := opts.HealthCheckTopic

	WithHealthCheckTopic("")(opts)

	assert.Equal(t, original, opts.HealthCheckTopic, "空字符串不应覆盖默认值")
}

func TestClientOptions_WithHealthCheckTopic_Default(t *testing.T) {
	opts := defaultOptions()

	assert.Equal(t, defaultHealthCheckTopic, opts.HealthCheckTopic)
}

// =============================================================================
// Client Interface Tests
// =============================================================================

func TestClientInterface_TypeCheck(t *testing.T) {
	// 编译时检查 clientWrapper 实现 Client 接口
	var _ Client = (*clientWrapper)(nil)
}
