package storage

import (
	"context"
	"errors"
	"time"

	"github.com/trusch/deadman-switch/pkg/config"
)

var (
	ErrNotFound = errors.New("not found")
)

type Storage interface {
	SetLastHeartbeat(ctx context.Context, key string, t time.Time) error
	GetLastHeartbeat(ctx context.Context, key string) (time.Time, error)

	SetAlarmActiveSince(ctx context.Context, key string, t time.Time) error
	GetAlarmActiveSince(ctx context.Context, key string) (time.Time, error)
	ClearAlarm(ctx context.Context, key string) error

	SetLastMessageSendTimestamp(ctx context.Context, key string, t time.Time) error
	GetLastMessageSendTimestamp(ctx context.Context, key string) (time.Time, error)

	GetServiceConfigs(ctx context.Context) (chan config.ServiceConfig, chan error)
	GetServiceConfig(ctx context.Context, id string) (config.ServiceConfig, error)
	SaveServiceConfig(ctx context.Context, svc config.ServiceConfig) error
	DeleteServiceConfig(ctx context.Context, id string) error
}
