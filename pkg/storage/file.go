package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog/log"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/trusch/deadman-switch/pkg/config"
)

func NewFileStorage(cfg config.ServerConfig) (Storage, error) {
	var fileCfg config.FileStorageConfig
	err := mapstructure.Decode(cfg.Storage.Config, &fileCfg)
	if err != nil {
		return nil, err
	}
	db, err := leveldb.OpenFile(fileCfg.File, nil)
	if err != nil {
		return nil, err
	}
	store := &fileStorage{db: db}
	for _, svc := range cfg.Services {
		err := store.SaveServiceConfig(context.Background(), svc)
		if err != nil {
			return nil, err
		}
	}
	return store, nil
}

type fileStorage struct {
	db *leveldb.DB
}

func (s *fileStorage) SetLastHeartbeat(ctx context.Context, key string, t time.Time) error {
	err := s.db.Put([]byte(filepath.Join("heartbeats", key)), []byte(t.Format(time.RFC3339)), nil)
	if err != nil {
		return err
	}
	return err
}

func (s *fileStorage) GetLastHeartbeat(ctx context.Context, key string) (time.Time, error) {
	resp, err := s.db.Get([]byte(filepath.Join("heartbeats", key)), nil)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, string(resp))
}

func (s *fileStorage) SetAlarmActiveSince(ctx context.Context, key string, t time.Time) error {
	err := s.db.Put([]byte(filepath.Join("alarms", key)), []byte(t.Format(time.RFC3339)), nil)
	if err != nil {
		return err
	}
	return err
}

func (s *fileStorage) GetAlarmActiveSince(ctx context.Context, key string) (time.Time, error) {
	resp, err := s.db.Get([]byte(filepath.Join("alarms", key)), nil)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, string(resp))
}

func (s *fileStorage) ClearAlarm(ctx context.Context, key string) error {
	err := s.db.Delete([]byte(filepath.Join("alarms", key)), nil)
	return err
}

func (s *fileStorage) SetLastMessageSendTimestamp(ctx context.Context, key string, t time.Time) error {
	err := s.db.Put([]byte(filepath.Join("lastMessage", key)), []byte(t.Format(time.RFC3339)), nil)
	if err != nil {
		return err
	}
	return err
}

func (s *fileStorage) GetLastMessageSendTimestamp(ctx context.Context, key string) (time.Time, error) {
	resp, err := s.db.Get([]byte(filepath.Join("lastMessage", key)), nil)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, string(resp))
}

func (s *fileStorage) SaveServiceConfig(ctx context.Context, svc config.ServiceConfig) error {
	bs, err := json.Marshal(svc)
	if err != nil {
		return err
	}
	err = s.db.Put([]byte(filepath.Join("services", svc.ID)), bs, nil)
	if err != nil {
		return err
	}
	return nil
}

func (s *fileStorage) DeleteServiceConfig(ctx context.Context, id string) error {
	err := s.db.Delete([]byte(filepath.Join("services", id)), nil)
	if err != nil {
		return err
	}
	return nil
}

func (s *fileStorage) GetServiceConfig(ctx context.Context, id string) (cfg config.ServiceConfig, err error) {
	resp, err := s.db.Get([]byte(filepath.Join("services", id)), nil)
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(resp, &cfg)
	if err != nil {
		return cfg, err
	}
	return cfg, nil
}

// GetServiceConfigs implements `config.Provider`
func (s *fileStorage) GetServiceConfigs(ctx context.Context) (configChannel chan config.ServiceConfig, errorChannel chan error) {
	configChannel = make(chan config.ServiceConfig, 32)
	errorChannel = make(chan error, 32)
	go func() {
		defer func() {
			defer close(configChannel)
			defer close(errorChannel)
		}()
		iterator := s.db.NewIterator(util.BytesPrefix([]byte("services")), nil)
		for iterator.Next() {
			var cfg config.ServiceConfig
			err := json.Unmarshal(iterator.Value(), &cfg)
			if err != nil {
				log.Error().Err(err).Str("data", string(iterator.Value())).Msg("failed to unmarshal")
				errorChannel <- err
				return
			}
			log.Debug().Str("key", string(iterator.Key())).Msg("read config from file")
			configChannel <- cfg
		}
	}()
	return
}
