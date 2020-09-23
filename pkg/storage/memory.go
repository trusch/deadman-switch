package storage

import (
	"context"
	"errors"
	"time"

	"github.com/trusch/deadman-switch/pkg/config"
)

func NewMemoryStorage(cfg config.ServerConfig) Storage {
	return &memoryStorage{
		cfg:         cfg,
		heartbeats:  make(map[string]time.Time),
		active:      make(map[string]time.Time),
		lastMessage: make(map[string]time.Time),
	}
}

type memoryStorage struct {
	cfg         config.ServerConfig
	heartbeats  map[string]time.Time
	active      map[string]time.Time
	lastMessage map[string]time.Time
}

func (s memoryStorage) SetLastHeartbeat(ctx context.Context, key string, t time.Time) error {
	s.heartbeats[key] = t
	return nil
}

func (s memoryStorage) GetLastHeartbeat(ctx context.Context, key string) (time.Time, error) {
	t, ok := s.heartbeats[key]
	if !ok {
		return t, ErrNotFound
	}
	return t, nil
}

func (s memoryStorage) SetAlarmActiveSince(ctx context.Context, key string, t time.Time) error {
	s.active[key] = t
	return nil
}

func (s memoryStorage) GetAlarmActiveSince(ctx context.Context, key string) (time.Time, error) {
	t, ok := s.active[key]
	if !ok {
		return t, ErrNotFound
	}
	return t, nil
}

func (s memoryStorage) SetLastMessageSendTimestamp(ctx context.Context, key string, t time.Time) error {
	s.lastMessage[key] = t
	return nil
}

func (s memoryStorage) GetLastMessageSendTimestamp(ctx context.Context, key string) (time.Time, error) {
	t, ok := s.lastMessage[key]
	if !ok {
		return t, ErrNotFound
	}
	return t, nil
}

func (s memoryStorage) ClearAlarm(ctx context.Context, key string) error {
	delete(s.active, key)
	return nil
}

func (s *memoryStorage) GetServiceConfig(ctx context.Context, id string) (config.ServiceConfig, error) {
	for _, svc := range s.cfg.Services {
		if svc.ID == id {
			return svc, nil
		}
	}
	return config.ServiceConfig{}, errors.New("not found")
}

// GetServiceConfigs implements `Provider` for the ServerConfig itself to serve static service configs
func (s *memoryStorage) GetServiceConfigs(ctx context.Context) (configChannel chan config.ServiceConfig, errorChannel chan error) {
	configChannel = make(chan config.ServiceConfig, 32)
	errorChannel = make(chan error, 32)
	go func() {
		defer func() {
			defer close(configChannel)
			defer close(errorChannel)
		}()
		for _, val := range s.cfg.Services {
			select {
			case <-ctx.Done():
				errorChannel <- ctx.Err()
				return
			default:
				configChannel <- val
			}
		}
	}()
	return
}

func (s *memoryStorage) SaveServiceConfig(ctx context.Context, svc config.ServiceConfig) error {
	s.cfg.Services = append(s.cfg.Services, svc)
	return nil
}

func (s *memoryStorage) DeleteServiceConfig(ctx context.Context, id string) error {
	for idx, val := range s.cfg.Services {
		if val.ID == id {
			s.cfg.Services = append(s.cfg.Services[:idx], s.cfg.Services[idx+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}
