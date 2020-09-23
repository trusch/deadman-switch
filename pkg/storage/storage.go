package storage

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
)

type Storage interface {
	SetTimestamp(ctx context.Context, key string, t time.Time) error
	GetTimestamp(ctx context.Context, key string) (time.Time, error)
}
