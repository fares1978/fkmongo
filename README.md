# fkmongo

A thin generic repository layer over the official MongoDB Go driver (v2).

```sh
go get github.com/fares1978/fkmongo@v0.1.0
go mod tidy
```

## Usage

```go
import (
	"github.com/fares1978/fkmongo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Supplier struct {
	ID    bson.ObjectID `bson:"_id,omitempty" json:"id"`
	Name  string        `bson:"name" json:"name"`
	Email string        `bson:"email" json:"email"`
}

// Embed BaseRepository and add collection-specific methods next to it.
type SupplierRepo struct {
	fkmongo.BaseRepository[Supplier]
}

func NewSupplierRepo(ctx context.Context, db *mongo.Database) (SupplierRepo, error) {
	r := SupplierRepo{fkmongo.NewBaseRepository[Supplier](db.Collection("suppliers"), "supplier")}
	err := r.EnsureIndexes(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return r, err
}

func (r SupplierRepo) Rename(ctx context.Context, id bson.ObjectID, name string) error {
	return r.UpdateByID(ctx, id, bson.M{"name": name})
}
```

Connecting (the app reads its own config):

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
client, err := fkmongo.Connect(ctx, os.Getenv("MONGO_URI"))
if err != nil { ... }
defer client.Disconnect(context.Background())
```

## Methods

| Area | Methods |
|---|---|
| Insert | `InsertOne` (returns `_id`), `InsertAndReturn`, `InsertMany` |
| Read | `FindOne`, `FindOneByID`, `Find`, `FindPage`, `Exists`, `ExistsByID`, `Count` |
| Update (`$set`) | `UpdateOne`, `UpdateByID`, `UpdateMany`, `Upsert`, `FindOneAndUpdate`, `FindByIDAndUpdate` |
| Update (any operator) | `UpdateOneRaw(ctx, filter, bson.M{"$inc": ...})` |
| Delete | `DeleteOne`, `DeleteByID`, `DeleteMany` |
| Other | `EnsureIndexes`, `WithTransaction`, `TypeToBson`, `StrToObjectID` |

Filters are `any` (`bson.M`, `bson.D`, ...). IDs are `any`, so string or UUID `_id`s work.
Read methods accept driver options, e.g. `options.Find().SetSort(...)`.
For anything else use `r.Coll` directly.

## Errors

```go
s, err := repo.FindOneByID(ctx, id)
switch {
case errors.Is(err, fkmongo.ErrNotFound):  // 404 — "supplier not found"
case errors.Is(err, fkmongo.ErrDuplicate): // 409 — "supplier with this email already exists"
case err != nil:                           // 500
}
```

`errors.As(err, &dup)` with `var dup *fkmongo.DuplicateKeyError` gives the colliding `Fields`.
Single-document updates and deletes return `ErrNotFound` when nothing matched.

## Transactions

Requires a replica set. Pass the `ctx` given to `fn` to every call.

```go
err := fkmongo.WithTransaction(ctx, client, func(ctx context.Context) error {
	if _, err := suppliers.InsertOne(ctx, &s); err != nil {
		return err
	}
	return audit.UpdateByID(ctx, auditID, bson.M{"last": s.Name})
})
```

## Tests

```sh
go test ./...                                                      # unit tests only
FKMONGO_TEST_URI='mongodb://localhost:27017/?directConnection=true' go test ./...
```

## Releasing a new version

1. Commit the changes and run the tests.

2. Pick the next version ([semver](https://semver.org); while on `v0`):

   | Change | Bump | Example |
   |---|---|---|
   | Bug fix, docs, internal change | patch | `v0.1.0` → `v0.1.1` |
   | New feature or breaking API change | minor | `v0.1.1` → `v0.2.0` |

   Check the latest tag with `git describe --tags --abbrev=0`.

3. Tag the commit and push the branch and the tag:

   ```sh
   git tag -a v0.1.1 -m "v0.1.1"
   git push origin main
   git push origin v0.1.1
   ```

4. Update the projects that use it:

   ```sh
   go get github.com/fares1978/fkmongo@v0.1.1   # or @latest
   go mod tidy
   ```

**Never move or delete a pushed tag.** The Go module proxy and checksum database
record each version permanently, so a version whose tag was moved fails to
install with a checksum mismatch. To fix a bad release, publish a new version instead.

## Migrating from the v1-driver version

| Before | After |
|---|---|
| `go.mongodb.org/mongo-driver/...` | `go.mongodb.org/mongo-driver/v2/...` |
| `primitive.ObjectID` | `bson.ObjectID` |
| `BaseRepository[T]{DB: db, Client: cl, Coll: c}` | `NewBaseRepository[T](c, "entity")` (use `r.Coll.Database()` / `.Client()` if needed) |
| `Connect()` reading `MONGO_URI` | `Connect(ctx, os.Getenv("MONGO_URI"))` |
| trailing `e string` argument | removed, set once via `Entity` |
| `ErrNoRecord` | `ErrNotFound` / `*NotFoundError` |
| `InsertOne(ctx, t, e) error` | `InsertOne(ctx, t) (id any, err error)` |
| `All(ctx)` / `AllWhere(ctx, f)` | `Find(ctx, bson.M{})` / `Find(ctx, f)` |
| `UpdateByFilter` | `UpdateOne` |
| `UpdateByIDAndReturn`, `FindAndUpdateByID` | `FindByIDAndUpdate` |
| `FindAndUpdateBy(ctx, k, v, u)` | `FindOneAndUpdate(ctx, bson.M{k: v}, u)` |
| `InsertUpdateOne` | `Upsert` |
| `DeleteMany(ctx, f) error`, `DeleteAll` | `DeleteMany(ctx, f) (int64, error)` |
| `UpdateVendorSyncDate` | move to your domain repo (`UpdateByID`) |
| `Decode`, `Repository` interface | removed |
