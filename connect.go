package fkmongo

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// Connect creates a client for uri and verifies it with a ping, bounded by ctx.
// opts are applied after the URI, so they override settings in it.
// The caller owns the client and should Disconnect it on shutdown.
func Connect(ctx context.Context, uri string, opts ...*options.ClientOptions) (*mongo.Client, error) {
	if uri == "" {
		return nil, errors.New("empty MongoDB URI")
	}
	all := append([]*options.ClientOptions{options.Client().ApplyURI(uri)}, opts...)
	client, err := mongo.Connect(all...)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		// Don't leak the client's background monitoring goroutines.
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping: %w", err)
	}
	return client, nil
}
