package xmongo

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type benchCollectionOps struct {
	docs      []any
	count     int64
	collName  string
	insertErr error
}

func (b *benchCollectionOps) CountDocuments(_ context.Context, _ any, _ ...options.Lister[options.CountOptions]) (int64, error) {
	return b.count, nil
}

func (b *benchCollectionOps) Find(_ context.Context, _ any, _ ...options.Lister[options.FindOptions]) (*mongo.Cursor, error) {
	return mongo.NewCursorFromDocuments(b.docs, nil, nil)
}

func (b *benchCollectionOps) InsertMany(_ context.Context, documents []any, _ ...options.Lister[options.InsertManyOptions]) (*mongo.InsertManyResult, error) {
	if b.insertErr != nil {
		return nil, b.insertErr
	}
	ids := make([]any, len(documents))
	for i := range documents {
		ids[i] = bson.NewObjectID()
	}
	return &mongo.InsertManyResult{InsertedIDs: ids}, nil
}

func (b *benchCollectionOps) Database() *mongo.Database {
	return nil
}

func (b *benchCollectionOps) Name() string {
	return b.collName
}

func BenchmarkFindPageInternal(b *testing.B) {
	docs := []any{
		bson.M{"id": 1, "name": "a"},
		bson.M{"id": 2, "name": "b"},
		bson.M{"id": 3, "name": "c"},
	}
	coll := &benchCollectionOps{
		docs:     docs,
		count:    int64(len(docs)),
		collName: "bench_collection",
	}
	w := &mongoWrapper{options: defaultOptions()}
	opts := PageOptions{Page: 1, PageSize: 2}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := w.findPageInternal(context.Background(), coll, bson.M{}, opts); err != nil {
			b.Fatalf("findPageInternal failed: %v", err)
		}
	}
}

func BenchmarkBulkWriteInternal(b *testing.B) {
	docs := make([]any, 100)
	for i := range docs {
		docs[i] = bson.M{"id": i}
	}
	coll := &benchCollectionOps{
		docs:     docs,
		count:    int64(len(docs)),
		collName: "bench_collection",
	}
	w := &mongoWrapper{options: defaultOptions()}
	opts := BulkOptions{BatchSize: 50}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := w.bulkWriteInternal(context.Background(), coll, docs, opts); err != nil {
			b.Fatalf("bulkWriteInternal failed: %v", err)
		}
	}
}
