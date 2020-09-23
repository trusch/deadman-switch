package concurrency

import "context"

type Client interface {
	IsLeader(ctx context.Context, id string) (bool, error)
	Lock(ctx context.Context, key string) error
}
