package fkmongo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Integration tests run only when FKMONGO_TEST_URI points at a MongoDB
// server. Transaction tests additionally need a replica set, e.g.
// mongodb://localhost:27017/?directConnection=true with mongod --replSet rs0.

type user struct {
	ID    bson.ObjectID `bson:"_id,omitempty"`
	Email string        `bson:"email"`
	Name  string        `bson:"name"`
	Age   int           `bson:"age"`
}

func newTestRepo(t *testing.T) (BaseRepository[user], *mongo.Client) {
	t.Helper()
	uri := os.Getenv("FKMONGO_TEST_URI")
	if uri == "" {
		t.Skip("FKMONGO_TEST_URI not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := Connect(ctx, uri)
	if err != nil {
		t.Fatal(err)
	}
	db := client.Database(fmt.Sprintf("fkmongo_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	repo := NewBaseRepository[user](db.Collection("users"), "user")
	err = repo.EnsureIndexes(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	return repo, client
}

func assertNotFound(t *testing.T, err error) {
	t.Helper()
	var nf *NotFoundError
	if !errors.Is(err, ErrNotFound) || !errors.As(err, &nf) || nf.Entity != "user" {
		t.Fatalf("want user NotFoundError, got %v", err)
	}
}

func assertDuplicate(t *testing.T, err error, fields ...string) {
	t.Helper()
	var dup *DuplicateKeyError
	if !errors.Is(err, ErrDuplicate) || !errors.As(err, &dup) {
		t.Fatalf("want DuplicateKeyError, got %v", err)
	}
	if !reflect.DeepEqual(dup.Fields, fields) {
		t.Fatalf("duplicate fields = %v, want %v", dup.Fields, fields)
	}
}

func TestConnectErrors(t *testing.T) {
	if _, err := Connect(context.Background(), ""); err == nil {
		t.Fatal("want error for empty URI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := Connect(ctx, "mongodb://127.0.0.1:1/?serverSelectionTimeoutMS=200"); err == nil {
		t.Fatal("want ping error for unreachable server")
	}
}

func TestInsertAndFind(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()

	got, err := repo.InsertAndReturn(ctx, &user{Email: "a@x.io", Name: "A", Age: 30})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID.IsZero() || got.Email != "a@x.io" {
		t.Fatalf("InsertAndReturn = %+v", got)
	}

	byID, err := repo.FindOneByID(ctx, got.ID)
	if err != nil || *byID != *got {
		t.Fatalf("FindOneByID = %+v, %v", byID, err)
	}

	_, err = repo.InsertOne(ctx, &user{Email: "a@x.io"})
	assertDuplicate(t, err, "email")

	_, err = repo.FindOne(ctx, bson.M{"email": "missing"})
	assertNotFound(t, err)

	if ids, err := repo.InsertMany(ctx, nil); err != nil || ids != nil {
		t.Fatalf("InsertMany(nil) = %v, %v", ids, err)
	}
	_, err = repo.InsertMany(ctx, []user{{Email: "b@x.io"}, {Email: "a@x.io"}})
	assertDuplicate(t, err, "email")

	none, err := repo.Find(ctx, bson.M{"age": 999})
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("Find with no matches = %#v, %v", none, err)
	}
}

func TestFindPageCountExists(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()

	var docs []user
	for i := range 7 {
		docs = append(docs, user{Email: fmt.Sprintf("u%d@x.io", i), Age: i})
	}
	if _, err := repo.InsertMany(ctx, docs); err != nil {
		t.Fatal(err)
	}

	page, err := repo.FindPage(ctx, bson.M{}, 3, 3, options.Find().SetSort(bson.D{{Key: "age", Value: 1}}))
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 7 || page.Pages != 3 || len(page.Items) != 1 || page.Items[0].Age != 6 {
		t.Fatalf("FindPage = %+v", page)
	}
	if _, err := repo.FindPage(ctx, bson.M{}, 1, 0); err == nil {
		t.Fatal("want error for size 0")
	}

	if n, err := repo.Count(ctx, bson.M{"age": bson.M{"$gte": 5}}); err != nil || n != 2 {
		t.Fatalf("Count = %d, %v", n, err)
	}
	if ok, err := repo.Exists(ctx, bson.M{"email": "u3@x.io"}); err != nil || !ok {
		t.Fatalf("Exists = %v, %v", ok, err)
	}
	if ok, err := repo.ExistsByID(ctx, bson.NewObjectID()); err != nil || ok {
		t.Fatalf("ExistsByID(missing) = %v, %v", ok, err)
	}
}

func TestUpdates(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()

	a, _ := repo.InsertAndReturn(ctx, &user{Email: "a@x.io", Age: 1})
	b, _ := repo.InsertAndReturn(ctx, &user{Email: "b@x.io", Age: 1})
	missing := bson.NewObjectID()

	if err := repo.UpdateByID(ctx, a.ID, bson.M{"name": "Alice"}); err != nil {
		t.Fatal(err)
	}
	assertNotFound(t, repo.UpdateByID(ctx, missing, bson.M{"name": "x"}))
	assertDuplicate(t, repo.UpdateByID(ctx, b.ID, bson.M{"email": "a@x.io"}), "email")

	if err := repo.UpdateOneRaw(ctx, bson.M{"_id": a.ID}, bson.M{"$inc": bson.M{"age": 10}}); err != nil {
		t.Fatal(err)
	}

	got, err := repo.FindByIDAndUpdate(ctx, a.ID, bson.M{"name": "Alicia"})
	if err != nil || got.Name != "Alicia" || got.Age != 11 {
		t.Fatalf("FindByIDAndUpdate = %+v, %v", got, err)
	}
	_, err = repo.FindByIDAndUpdate(ctx, missing, bson.M{"name": "x"})
	assertNotFound(t, err)
	_, err = repo.FindByIDAndUpdate(ctx, b.ID, bson.M{"email": "a@x.io"})
	assertDuplicate(t, err, "email")

	if n, err := repo.UpdateMany(ctx, bson.M{}, bson.M{"name": "all"}); err != nil || n != 2 {
		t.Fatalf("UpdateMany = %d, %v", n, err)
	}

	if err := repo.Upsert(ctx, bson.M{"email": "c@x.io"}, bson.M{"age": 5}); err != nil {
		t.Fatal(err)
	}
	if c, err := repo.FindOne(ctx, bson.M{"email": "c@x.io"}); err != nil || c.Age != 5 {
		t.Fatalf("after Upsert = %+v, %v", c, err)
	}
}

func TestDeletes(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()

	a, _ := repo.InsertAndReturn(ctx, &user{Email: "a@x.io"})
	_, _ = repo.InsertMany(ctx, []user{{Email: "b@x.io"}, {Email: "c@x.io"}})

	if err := repo.DeleteByID(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	assertNotFound(t, repo.DeleteByID(ctx, a.ID))
	if n, err := repo.DeleteMany(ctx, bson.M{}); err != nil || n != 2 {
		t.Fatalf("DeleteMany = %d, %v", n, err)
	}
}

func TestWithTransaction(t *testing.T) {
	repo, client := newTestRepo(t)
	ctx := context.Background()

	var hello bson.M
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		t.Fatal(err)
	}
	if _, ok := hello["setName"]; !ok {
		t.Skip("transactions need a replica set")
	}

	err := WithTransaction(ctx, client, func(ctx context.Context) error {
		_, err := repo.InsertOne(ctx, &user{Email: "commit@x.io"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	boom := errors.New("boom")
	err = WithTransaction(ctx, client, func(ctx context.Context) error {
		if _, err := repo.InsertOne(ctx, &user{Email: "rollback@x.io"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("WithTransaction error = %v, want boom", err)
	}

	if ok, _ := repo.Exists(ctx, bson.M{"email": "commit@x.io"}); !ok {
		t.Error("committed insert missing")
	}
	if ok, _ := repo.Exists(ctx, bson.M{"email": "rollback@x.io"}); ok {
		t.Error("aborted insert was persisted")
	}
}
