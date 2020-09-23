package config

type ServerConfig struct {
	HTTPListenAddress string          `json:"httpListenAddress"`
	CheckInterval     Duration        `json:"checkInterval"`
	Storage           StorageConfig   `json:"storage"`
	Services          []ServiceConfig `json:"services"`
}

type ServiceConfig struct {
	ID       string          `json:"id"`
	Timeout  Duration        `json:"timeout"`
	Webhooks []WebhookConfig `json:"webhooks"`
}

type WebhookConfig struct {
	URL    string `json:"url"`
	Method string `json:"method"`
	Body   string `json:"body"`
}

type StorageConfig struct {
	Type   StorageType            `json:"type"`
	Config map[string]interface{} `json:"config"`
}

type StorageType string

const (
	StorageTypeMemory     StorageType = "memory"
	StorageTypeEtcd       StorageType = "etcd"
	StorageTypePostgresql StorageType = "postgresql"
)
