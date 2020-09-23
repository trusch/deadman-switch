package storage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/trusch/deadman-switch/pkg/config"
	"go.etcd.io/etcd/clientv3"
)

func NewEtcdStorage(ctx context.Context, cli *clientv3.Client, prefix string) (Storage, error) {
	lease, err := cli.Grant(ctx, 5)
	if err != nil {
		return nil, err
	}
	ch, err := cli.KeepAlive(ctx, lease.ID)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				continue
			}
		}
	}()
	return &etcdStorage{
		client: cli,
		prefix: prefix,
		lease:  lease.ID,
	}, nil
}

type etcdStorage struct {
	client *clientv3.Client
	prefix string
	lease  clientv3.LeaseID
}

func (s *etcdStorage) SetLastHeartbeat(ctx context.Context, key string, t time.Time) error {
	_, err := s.client.KV.Put(ctx, filepath.Join(s.prefix, "heartbeats", key), t.Format(time.RFC3339))
	if err != nil {
		return err
	}
	return err
}

func (s *etcdStorage) GetLastHeartbeat(ctx context.Context, key string) (time.Time, error) {
	resp, err := s.client.KV.Get(ctx, filepath.Join(s.prefix, "heartbeats", key))
	if err != nil {
		return time.Time{}, err
	}
	if len(resp.Kvs) == 0 {
		return time.Time{}, ErrNotFound
	}
	return time.Parse(time.RFC3339, string(resp.Kvs[0].Value))
}

func (s *etcdStorage) SetAlarmActiveSince(ctx context.Context, key string, t time.Time) error {
	_, err := s.client.KV.Put(ctx, filepath.Join(s.prefix, "alarms", key), t.Format(time.RFC3339))
	if err != nil {
		return err
	}
	return err
}

func (s *etcdStorage) GetAlarmActiveSince(ctx context.Context, key string) (time.Time, error) {
	resp, err := s.client.KV.Get(ctx, filepath.Join(s.prefix, "alarms", key))
	if err != nil {
		return time.Time{}, err
	}
	if len(resp.Kvs) == 0 {
		return time.Time{}, ErrNotFound
	}
	return time.Parse(time.RFC3339, string(resp.Kvs[0].Value))
}

func (s *etcdStorage) ClearAlarm(ctx context.Context, key string) error {
	_, err := s.client.KV.Delete(ctx, filepath.Join(s.prefix, "alarms", key))
	return err
}

func (s *etcdStorage) SetLastMessageSendTimestamp(ctx context.Context, key string, t time.Time) error {
	_, err := s.client.KV.Put(ctx, filepath.Join(s.prefix, "lastMessage", key), t.Format(time.RFC3339))
	if err != nil {
		return err
	}
	return err
}

func (s *etcdStorage) GetLastMessageSendTimestamp(ctx context.Context, key string) (time.Time, error) {
	resp, err := s.client.KV.Get(ctx, filepath.Join(s.prefix, "lastMessage", key))
	if err != nil {
		return time.Time{}, err
	}
	if len(resp.Kvs) == 0 {
		return time.Time{}, ErrNotFound
	}
	return time.Parse(time.RFC3339, string(resp.Kvs[0].Value))
}

func (s *etcdStorage) SaveServiceConfig(ctx context.Context, svc config.ServiceConfig) error {
	bs, err := json.Marshal(svc)
	if err != nil {
		return err
	}
	_, err = s.client.KV.Put(ctx, filepath.Join(s.prefix, "services", svc.ID), string(bs))
	if err != nil {
		return err
	}
	return nil
}

func (s *etcdStorage) DeleteServiceConfig(ctx context.Context, id string) error {
	_, err := s.client.KV.Delete(ctx, filepath.Join(s.prefix, "services", id))
	if err != nil {
		return err
	}
	return nil
}

func (s *etcdStorage) GetServiceConfig(ctx context.Context, id string) (cfg config.ServiceConfig, err error) {
	resp, err := s.client.KV.Get(ctx, filepath.Join(s.prefix, "services", id))
	if err != nil {
		return cfg, err
	}
	if len(resp.Kvs) < 1 {
		return cfg, errors.New("not found")
	}
	err = json.Unmarshal(resp.Kvs[0].Value, &cfg)
	if err != nil {
		return cfg, err
	}
	return cfg, nil
}

// GetServiceConfigs implements `config.Provider`
func (s *etcdStorage) GetServiceConfigs(ctx context.Context) (configChannel chan config.ServiceConfig, errorChannel chan error) {
	configChannel = make(chan config.ServiceConfig, 32)
	errorChannel = make(chan error, 32)
	go func() {
		defer func() {
			defer close(configChannel)
			defer close(errorChannel)
		}()
		resp, err := s.client.KV.Get(ctx, filepath.Join(s.prefix, "services"), clientv3.WithPrefix())
		if err != nil {
			errorChannel <- err
			return
		}
		for _, val := range resp.Kvs {
			select {
			case <-ctx.Done():
				errorChannel <- ctx.Err()
				return
			default:
				var cfg config.ServiceConfig
				err = json.Unmarshal(val.Value, &cfg)
				if err != nil {
					log.Error().Err(err).Str("data", string(val.Value)).Msg("failed to unmarshal")
					errorChannel <- err
					return
				}
				log.Debug().Str("key", string(val.Key)).Msg("read config from etcd")
				configChannel <- cfg
			}
		}
	}()
	return
}
