package storage

import (
	"context"
	"time"
)

func NewMemoryStorage() Storage {
	return make(memoryStorage)
}

type memoryStorage map[string]time.Time

func (s memoryStorage) SetTimestamp(ctx context.Context, key string, t time.Time) error {
	s[key] = t
	return nil
}

func (s memoryStorage) GetTimestamp(ctx context.Context, key string) (time.Time, error) {
	t, ok := s[key]
	if !ok {
		return t, ErrNotFound
	}
	return t, nil
}
