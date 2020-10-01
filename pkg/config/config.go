package config

import (
	"errors"

	"github.com/mitchellh/mapstructure"
)

type ServerConfig struct {
	HTTPListenAddress string          `json:"listen"`
	ID                string          `json:"id"`
	Username          string          `json:"username"`
	Password          string          `json:"password"`
	CheckInterval     Duration        `json:"checkInterval"`
	Storage           StorageConfig   `json:"storage"`
	Services          []ServiceConfig `json:"services"`
}

type ServiceConfig struct {
	ID                    string               `json:"id"`
	Token                 string               `json:"token"`
	Timeout               Duration             `json:"timeout"`
	Debounce              Duration             `json:"debounce"`
	AlertNotifications    []NotificationConfig `json:"alertNotifications"`
	RecoveryNotifications []NotificationConfig `json:"recoveryNotifications"`
}

type NotificationConfig struct {
	Type   NotificationType
	Config interface{}
}

type WebhookConfig struct {
	URL     string              `json:"url"`
	Method  string              `json:"method"`
	Body    string              `json:"body"`
	Headers map[string][]string `json:"headers"`
}

type SlackConfig struct {
	Token         string `json:"token"`
	Channel       string `json:"channel"`
	MessageFields []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"messageFields"`
}

type StorageConfig struct {
	Type   StorageType `json:"type"`
	Config interface{} `json:"config"`
}

type EtcdStorageConfig struct {
	Endpoints []string `json:"endpoints"`
}

type FileStorageConfig struct {
	File string `json:"file"`
}

type StorageType string

const (
	StorageTypeMemory StorageType = "memory"
	StorageTypeEtcd   StorageType = "etcd"
	StorageTypeFile   StorageType = "file"
)

type NotificationType string

const (
	NotificationTypeWebhook NotificationType = "webhook"
	NotificationTypeSlack   NotificationType = "slack"
)

func (n NotificationConfig) GetWebhookConfig() (cfg WebhookConfig, err error) {
	if n.Type != NotificationTypeWebhook {
		return cfg, errors.New("this is not a webhook config")
	}
	err = mapstructure.Decode(n.Config, &cfg)
	return cfg, err
}

func (n NotificationConfig) GetSlackConfig() (cfg SlackConfig, err error) {
	if n.Type != NotificationTypeSlack {
		return cfg, errors.New("this is not a slack config")
	}
	err = mapstructure.Decode(n.Config, &cfg)
	return cfg, err
}
