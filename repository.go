package fkmongo

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// BaseRepository provides typed CRUD operations on a collection whose
// documents decode into T. Embed it in a domain repository and use Coll
// directly for anything not covered here.
//
// Methods that take a set argument wrap it in {"$set": set}; it can be a
// bson.M, bson.D or a struct with bson tags. Use UpdateOneRaw for other
// update operators.
type BaseRepository[T any] struct {
	Coll *mongo.Collection
	// Entity names the document type in error messages, e.g. "supplier not found".
	Entity string
}

// NewBaseRepository returns a repository for coll. entity is used in error
// messages.
func NewBaseRepository[T any](coll *mongo.Collection, entity string) BaseRepository[T] {
	return BaseRepository[T]{Coll: coll, Entity: entity}
}

// Page is one page of results returned by FindPage.
type Page[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
	Page  int64 `json:"page"`
	Size  int64 `json:"size"`
	Pages int64 `json:"pages"`
}

func (r BaseRepository[T]) err(err error) error {
	return mapError(r.Entity, err)
}

// EnsureIndexes creates the given indexes. Creating an index that already
// exists with the same definition is a no-op.
func (r BaseRepository[T]) EnsureIndexes(ctx context.Context, models ...mongo.IndexModel) error {
	if len(models) == 0 {
		return nil
	}
	_, err := r.Coll.Indexes().CreateMany(ctx, models)
	return err
}

// InsertOne inserts doc and returns its _id.
func (r BaseRepository[T]) InsertOne(ctx context.Context, doc *T) (any, error) {
	res, err := r.Coll.InsertOne(ctx, doc)
	if err != nil {
		return nil, r.err(err)
	}
	return res.InsertedID, nil
}

// InsertAndReturn inserts doc and returns it as stored, including a
// driver-generated _id.
func (r BaseRepository[T]) InsertAndReturn(ctx context.Context, doc *T) (*T, error) {
	id, err := r.InsertOne(ctx, doc)
	if err != nil {
		return nil, err
	}
	return r.FindOneByID(ctx, id)
}

// InsertMany inserts docs and returns their _ids. An empty slice is a no-op.
func (r BaseRepository[T]) InsertMany(ctx context.Context, docs []T) ([]any, error) {
	if len(docs) == 0 {
		return nil, nil
	}
	res, err := r.Coll.InsertMany(ctx, docs)
	if err != nil {
		return nil, r.err(err)
	}
	return res.InsertedIDs, nil
}

// FindOne returns the first document matching filter, or a NotFoundError.
func (r BaseRepository[T]) FindOne(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) (*T, error) {
	t := new(T)
	if err := r.Coll.FindOne(ctx, filter, opts...).Decode(t); err != nil {
		return nil, r.err(err)
	}
	return t, nil
}

// FindOneByID returns the document with the given _id, or a NotFoundError.
func (r BaseRepository[T]) FindOneByID(ctx context.Context, id any, opts ...options.Lister[options.FindOneOptions]) (*T, error) {
	return r.FindOne(ctx, bson.M{"_id": id}, opts...)
}

// Find returns all documents matching filter. The result is never nil, so it
// encodes to [] rather than null in JSON.
func (r BaseRepository[T]) Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) ([]T, error) {
	cur, err := r.Coll.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	docs := []T{}
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

// FindPage returns page number page (starting at 1) of size documents
// matching filter, plus the total match count. Pass options.Find().SetSort
// in opts for a stable order.
func (r BaseRepository[T]) FindPage(ctx context.Context, filter any, page, size int64, opts ...options.Lister[options.FindOptions]) (Page[T], error) {
	if size < 1 {
		return Page[T]{}, errors.New("fkmongo: page size must be positive")
	}
	if page < 1 {
		page = 1
	}
	total, err := r.Count(ctx, filter)
	if err != nil {
		return Page[T]{}, err
	}
	opts = append(opts, options.Find().SetSkip((page-1)*size).SetLimit(size))
	items, err := r.Find(ctx, filter, opts...)
	if err != nil {
		return Page[T]{}, err
	}
	return Page[T]{
		Items: items,
		Total: total,
		Page:  page,
		Size:  size,
		Pages: (total + size - 1) / size,
	}, nil
}

// Exists reports whether any document matches filter.
func (r BaseRepository[T]) Exists(ctx context.Context, filter any) (bool, error) {
	opts := options.FindOne().SetProjection(bson.M{"_id": 1})
	err := r.Coll.FindOne(ctx, filter, opts).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ExistsByID reports whether a document with the given _id exists.
func (r BaseRepository[T]) ExistsByID(ctx context.Context, id any) (bool, error) {
	return r.Exists(ctx, bson.M{"_id": id})
}

// Count returns the number of documents matching filter.
func (r BaseRepository[T]) Count(ctx context.Context, filter any) (int64, error) {
	return r.Coll.CountDocuments(ctx, filter)
}

// UpdateOne applies set to the first document matching filter, or returns a
// NotFoundError if none matched.
func (r BaseRepository[T]) UpdateOne(ctx context.Context, filter, set any) error {
	return r.UpdateOneRaw(ctx, filter, bson.M{"$set": set})
}

// UpdateByID applies set to the document with the given _id, or returns a
// NotFoundError if it doesn't exist.
func (r BaseRepository[T]) UpdateByID(ctx context.Context, id, set any) error {
	return r.UpdateOne(ctx, bson.M{"_id": id}, set)
}

// UpdateOneRaw applies a full update document, such as
// bson.M{"$inc": bson.M{"views": 1}}, to the first document matching filter,
// or returns a NotFoundError if none matched.
func (r BaseRepository[T]) UpdateOneRaw(ctx context.Context, filter, update any) error {
	res, err := r.Coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return r.err(err)
	}
	if res.MatchedCount == 0 {
		return &NotFoundError{Entity: r.Entity}
	}
	return nil
}

// UpdateMany applies set to every document matching filter and returns how
// many matched.
func (r BaseRepository[T]) UpdateMany(ctx context.Context, filter, set any) (int64, error) {
	res, err := r.Coll.UpdateMany(ctx, filter, bson.M{"$set": set})
	if err != nil {
		return 0, r.err(err)
	}
	return res.MatchedCount, nil
}

// Upsert applies set to the first document matching filter, inserting a new
// document built from filter and set if none matched.
func (r BaseRepository[T]) Upsert(ctx context.Context, filter, set any) error {
	_, err := r.Coll.UpdateOne(ctx, filter, bson.M{"$set": set}, options.UpdateOne().SetUpsert(true))
	return r.err(err)
}

// FindOneAndUpdate atomically applies set to the first document matching
// filter and returns the updated document, or a NotFoundError.
func (r BaseRepository[T]) FindOneAndUpdate(ctx context.Context, filter, set any) (*T, error) {
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	t := new(T)
	if err := r.Coll.FindOneAndUpdate(ctx, filter, bson.M{"$set": set}, opts).Decode(t); err != nil {
		return nil, r.err(err)
	}
	return t, nil
}

// FindByIDAndUpdate atomically applies set to the document with the given
// _id and returns the updated document, or a NotFoundError.
func (r BaseRepository[T]) FindByIDAndUpdate(ctx context.Context, id, set any) (*T, error) {
	return r.FindOneAndUpdate(ctx, bson.M{"_id": id}, set)
}

// DeleteOne deletes the first document matching filter, or returns a
// NotFoundError if none matched.
func (r BaseRepository[T]) DeleteOne(ctx context.Context, filter any) error {
	res, err := r.Coll.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return &NotFoundError{Entity: r.Entity}
	}
	return nil
}

// DeleteByID deletes the document with the given _id, or returns a
// NotFoundError if it doesn't exist.
func (r BaseRepository[T]) DeleteByID(ctx context.Context, id any) error {
	return r.DeleteOne(ctx, bson.M{"_id": id})
}

// DeleteMany deletes every document matching filter and returns how many were
// deleted. Pass bson.M{} to empty the collection.
func (r BaseRepository[T]) DeleteMany(ctx context.Context, filter any) (int64, error) {
	res, err := r.Coll.DeleteMany(ctx, filter)
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}
