package storage

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"go.etcd.io/etcd/clientv3"
)

func NewEtcdStorage(endpoints []string) (Storage, error) {
	log.Info().Msgf("connecting to etcd at %v", endpoints)
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 2 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return &etcdStorage{
		client: cli,
	}, nil
}

type etcdStorage struct {
	client *clientv3.Client
}

func (s etcdStorage) SetTimestamp(ctx context.Context, key string, t time.Time) error {
	_, err := s.client.KV.Put(ctx, key, t.Format(time.RFC3339))
	return err
}

func (s etcdStorage) GetTimestamp(ctx context.Context, key string) (time.Time, error) {
	resp, err := s.client.KV.Get(ctx, key)
	if err != nil {
		return time.Time{}, err
	}
	if len(resp.Kvs) == 0 {
		return time.Time{}, ErrNotFound
	}
	return time.Parse(time.RFC3339, string(resp.Kvs[0].Value))
}
