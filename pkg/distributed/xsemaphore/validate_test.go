package xsemaphore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateResource(t *testing.T) {
	tests := []struct {
		name     string
		resource string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid simple name",
			resource: "inference-task",
			wantErr:  false,
		},
		{
			name:     "valid name with numbers",
			resource: "task-123",
			wantErr:  false,
		},
		{
			name:     "valid name with underscore",
			resource: "my_resource_name",
			wantErr:  false,
		},
		{
			name:     "valid name with dots",
			resource: "api.external.call",
			wantErr:  false,
		},
		{
			name:     "empty string",
			resource: "",
			wantErr:  true,
			errMsg:   "cannot be empty",
		},
		{
			name:     "contains left brace",
			resource: "test{resource",
			wantErr:  true,
			errMsg:   "cannot contain '{'",
		},
		{
			name:     "contains right brace",
			resource: "test}resource",
			wantErr:  true,
			errMsg:   "cannot contain '}'",
		},
		{
			name:     "contains colon",
			resource: "test:resource",
			wantErr:  true,
			errMsg:   "cannot contain ':'",
		},
		{
			name:     "contains hash tag pattern",
			resource: "{hashtag}",
			wantErr:  true,
			errMsg:   "cannot contain '{'",
		},
		{
			name:     "contains multiple invalid chars",
			resource: "test:{invalid}",
			wantErr:  true,
			errMsg:   "cannot contain ':'", // 第一个无效字符
		},
		{
			name:     "exceeds max length",
			resource: string(make([]byte, MaxResourceLength+1)),
			wantErr:  true,
			errMsg:   "exceeds max length",
		},
		{
			name: "at max length is valid",
			resource: func() string {
				b := make([]byte, MaxResourceLength)
				for i := range b {
					b[i] = 'a'
				}
				return string(b)
			}(),
			wantErr: false,
		},
		{
			name:     "contains space",
			resource: "test resource",
			wantErr:  true,
			errMsg:   "whitespace",
		},
		{
			name:     "contains tab",
			resource: "test\tresource",
			wantErr:  true,
			errMsg:   "whitespace",
		},
		{
			name:     "contains newline",
			resource: "test\nresource",
			wantErr:  true,
			errMsg:   "whitespace",
		},
		{
			name:     "contains carriage return",
			resource: "test\rresource",
			wantErr:  true,
			errMsg:   "whitespace",
		},
		{
			name:     "leading space",
			resource: " resource",
			wantErr:  true,
			errMsg:   "whitespace",
		},
		{
			name:     "trailing space",
			resource: "resource ",
			wantErr:  true,
			errMsg:   "whitespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResource(tt.resource)
			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidResource)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTryAcquire_InvalidResourceChars 测试 TryAcquire 拒绝无效资源名
func TestTryAcquire_InvalidResourceChars(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := t.Context()

	invalidResources := []string{
		"test{resource",
		"test}resource",
		"test:resource",
		"{hashtag}",
	}

	for _, resource := range invalidResources {
		t.Run(resource, func(t *testing.T) {
			permit, err := sem.TryAcquire(ctx, resource, WithCapacity(10))
			assert.ErrorIs(t, err, ErrInvalidResource)
			assert.Nil(t, permit)
		})
	}
}

// TestQuery_InvalidResourceChars 测试 Query 拒绝无效资源名
func TestQuery_InvalidResourceChars(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := t.Context()

	info, err := sem.Query(ctx, "test:invalid", QueryWithCapacity(10))
	assert.ErrorIs(t, err, ErrInvalidResource)
	assert.Nil(t, info)
}

// TestLocalSemaphore_InvalidResourceChars 测试本地信号量拒绝无效资源名
func TestLocalSemaphore_InvalidResourceChars(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)

	ctx := t.Context()

	permit, err := sem.TryAcquire(ctx, "test{invalid", WithCapacity(10))
	assert.ErrorIs(t, err, ErrInvalidResource)
	assert.Nil(t, permit)

	info, err := sem.Query(ctx, "test}invalid", QueryWithCapacity(10))
	assert.ErrorIs(t, err, ErrInvalidResource)
	assert.Nil(t, info)
}

// =============================================================================
// KeyPrefix 校验测试
// =============================================================================

func TestValidateKeyPrefix(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		wantErr bool
	}{
		{
			name:    "valid simple prefix",
			prefix:  "xsemaphore:",
			wantErr: false,
		},
		{
			name:    "valid prefix with dots",
			prefix:  "app.semaphore:",
			wantErr: false,
		},
		{
			name:    "valid prefix with underscores",
			prefix:  "my_app_sem:",
			wantErr: false,
		},
		{
			name:    "empty prefix is valid",
			prefix:  "",
			wantErr: false,
		},
		{
			name:    "contains left brace",
			prefix:  "app:{env}:",
			wantErr: true,
		},
		{
			name:    "contains right brace",
			prefix:  "app:}env:",
			wantErr: true,
		},
		{
			name:    "only left brace",
			prefix:  "test{",
			wantErr: true,
		},
		{
			name:    "only right brace",
			prefix:  "test}",
			wantErr: true,
		},
		{
			name:    "hash tag pattern",
			prefix:  "{cluster}:",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateKeyPrefix(tt.prefix)
			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidKeyPrefix)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestWithKeyPrefix_InvalidPrefix 测试 New() 对无效前缀返回错误
func TestWithKeyPrefix_InvalidPrefix(t *testing.T) {
	_, client := setupRedis(t)

	// Fail-fast: 无效前缀应在 New() 时返回错误
	_, err := New(client, WithKeyPrefix("invalid{prefix}:"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidKeyPrefix)
}

// TestWithKeyPrefix_ValidPrefix 测试 WithKeyPrefix 接受有效前缀
func TestWithKeyPrefix_ValidPrefix(t *testing.T) {
	_, client := setupRedis(t)

	// 使用有效前缀
	sem, err := New(client, WithKeyPrefix("myapp:sem:"))
	require.NoError(t, err)
	defer closeSemaphore(t, sem)

	ctx := t.Context()
	permit, err := sem.TryAcquire(ctx, "test-resource", WithCapacity(10))
	require.NoError(t, err)
	require.NotNil(t, permit)
	releasePermit(t, ctx, permit)
}

// =============================================================================
// 租户 ID 校验测试
// =============================================================================

func TestValidateTenantID(t *testing.T) {
	tests := []struct {
		name     string
		tenantID string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "empty is valid",
			tenantID: "",
			wantErr:  false,
		},
		{
			name:     "valid simple id",
			tenantID: "tenant-123",
			wantErr:  false,
		},
		{
			name:     "valid with dots",
			tenantID: "org.team.user",
			wantErr:  false,
		},
		{
			name:     "contains left brace",
			tenantID: "tenant{123}",
			wantErr:  true,
			errMsg:   "cannot contain '{'",
		},
		{
			name:     "contains right brace",
			tenantID: "tenant}123",
			wantErr:  true,
			errMsg:   "cannot contain '}'",
		},
		{
			name:     "contains colon",
			tenantID: "tenant:123",
			wantErr:  true,
			errMsg:   "cannot contain ':'",
		},
		{
			name:     "contains space",
			tenantID: "tenant 123",
			wantErr:  true,
			errMsg:   "whitespace",
		},
		{
			name:     "contains tab",
			tenantID: "tenant\t123",
			wantErr:  true,
			errMsg:   "whitespace",
		},
		{
			name:     "exceeds max length",
			tenantID: string(make([]byte, MaxTenantIDLength+1)),
			wantErr:  true,
			errMsg:   "exceeds max length",
		},
		{
			name: "at max length is valid",
			tenantID: func() string {
				b := make([]byte, MaxTenantIDLength)
				for i := range b {
					b[i] = 'a'
				}
				return string(b)
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTenantID(tt.tenantID)
			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidTenantID)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTryAcquire_InvalidTenantID 测试 TryAcquire 拒绝无效租户 ID
func TestTryAcquire_InvalidTenantID(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := t.Context()

	invalidTenantIDs := []string{
		"tenant{123}",
		"tenant}123",
		"tenant:123",
		"tenant 123",
	}

	for _, tenantID := range invalidTenantIDs {
		t.Run(tenantID, func(t *testing.T) {
			permit, err := sem.TryAcquire(ctx, "resource",
				WithCapacity(10),
				WithTenantID(tenantID),
				WithTenantQuota(5),
			)
			assert.ErrorIs(t, err, ErrInvalidTenantID)
			assert.Nil(t, permit)
		})
	}
}

// TestQuery_InvalidTenantID 测试 Query 拒绝无效租户 ID
func TestQuery_InvalidTenantID(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := t.Context()

	info, err := sem.Query(ctx, "resource",
		QueryWithCapacity(10),
		QueryWithTenantID("tenant:invalid"),
	)
	assert.ErrorIs(t, err, ErrInvalidTenantID)
	assert.Nil(t, info)
}
