package xmongo

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// =============================================================================
// Mock 实现 - 用于单元测试
// =============================================================================

// mockClientOps 实现 clientOperations 接口
type mockClientOps struct {
	pingErr            error
	pingCount          int
	disconnectErr      error
	disconnected       bool
	sessionsInProgress int
}

func (m *mockClientOps) Ping(_ context.Context, _ *readpref.ReadPref) error {
	m.pingCount++
	return m.pingErr
}

func (m *mockClientOps) Disconnect(_ context.Context) error {
	m.disconnected = true
	return m.disconnectErr
}

func (m *mockClientOps) NumberSessionsInProgress() int {
	return m.sessionsInProgress
}

// mockCollectionOps 实现 collectionOperations 接口
type mockCollectionOps struct {
	countResult  int64
	countErr     error
	findCursor   *mongo.Cursor
	findErr      error
	insertResult *mongo.InsertManyResult
	insertErr    error
	collName     string
	mockDB       *mongo.Database
}

func (m *mockCollectionOps) CountDocuments(_ context.Context, _ any, _ ...options.Lister[options.CountOptions]) (int64, error) {
	return m.countResult, m.countErr
}

func (m *mockCollectionOps) Find(_ context.Context, _ any, _ ...options.Lister[options.FindOptions]) (*mongo.Cursor, error) {
	return m.findCursor, m.findErr
}

func (m *mockCollectionOps) InsertMany(_ context.Context, documents []any, _ ...options.Lister[options.InsertManyOptions]) (*mongo.InsertManyResult, error) {
	if m.insertErr != nil {
		return nil, m.insertErr
	}
	if m.insertResult != nil {
		return m.insertResult, nil
	}
	// 默认返回插入的文档 ID
	ids := make([]any, len(documents))
	for i := range documents {
		ids[i] = bson.NewObjectID()
	}
	return &mongo.InsertManyResult{InsertedIDs: ids}, nil
}

func (m *mockCollectionOps) Database() *mongo.Database {
	return m.mockDB
}

func (m *mockCollectionOps) Name() string {
	return m.collName
}

// =============================================================================
// 辅助构造函数
// =============================================================================

// newMockClientOps 创建一个新的 mock 客户端操作
func newMockClientOps() *mockClientOps {
	return &mockClientOps{}
}

// newMockCollectionOps 创建一个新的 mock 集合操作
func newMockCollectionOps() *mockCollectionOps {
	return &mockCollectionOps{
		collName: "test_collection",
	}
}

// =============================================================================
// Mock Cursor 说明
// =============================================================================

// 注意：mongo.Cursor 不是接口，无法直接 mock
// 我们通过注入 mockCollectionOps.findErr 来测试错误路径
// 对于成功路径，需要集成测试或使用 mtest

// =============================================================================
// 错误定义
// =============================================================================

var (
	errMockPing       = errors.New("mock ping error")
	errMockDisconnect = errors.New("mock disconnect error")
	errMockCount      = errors.New("mock count error")
	errMockFind       = errors.New("mock find error")
	errMockInsert     = errors.New("mock insert error")
)
