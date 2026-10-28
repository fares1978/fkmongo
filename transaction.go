package fkmongo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// WithTransaction runs fn in a transaction, committing if fn returns nil and
// aborting otherwise. Transient errors are retried, so fn may run more than
// once. Repository calls join the transaction only when they are passed the
// ctx given to fn. Transactions require a replica set or sharded cluster.
func WithTransaction(ctx context.Context, client *mongo.Client, fn func(ctx context.Context) error) error {
	sess, err := client.StartSession()
	if err != nil {
		return err
	}
	defer sess.EndSession(ctx)
	_, err = sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		return nil, fn(ctx)
	})
	return err
}
