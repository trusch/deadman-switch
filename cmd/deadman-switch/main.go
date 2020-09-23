package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/ghodss/yaml"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/pflag"
	"github.com/trusch/deadman-switch/pkg/checker"
	"github.com/trusch/deadman-switch/pkg/concurrency"
	"github.com/trusch/deadman-switch/pkg/config"
	"github.com/trusch/deadman-switch/pkg/notifier"
	"github.com/trusch/deadman-switch/pkg/queue"
	"github.com/trusch/deadman-switch/pkg/server"
	"github.com/trusch/deadman-switch/pkg/storage"
	"go.etcd.io/etcd/clientv3"
)

var (
	configFile      = pflag.StringP("config", "c", "config.yaml", "config file")
	showVersion     = pflag.BoolP("version", "v", false, "show version")
	logLevel        = pflag.String("log-level", "info", "log level")
	logFormat       = pflag.String("log-format", "json", "log format ('json' or 'console')")
	Version, Commit string
)

func main() {
	ctx := context.Background()

	pflag.Parse()

	if *showVersion {
		fmt.Printf("Version: %s\nCommit: %s\n", Version, Commit)
		os.Exit(0)
	}

	lvl, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to parse --log-level flag")
	}
	zerolog.SetGlobalLevel(lvl)

	switch *logFormat {
	case "json":
	case "console":
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	default:
		log.Fatal().Str("format", *logFormat).Msg("unknown log format")
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal().
			Err(err).
			Str("file", *configFile).
			Msg("failed to load config")
	}

	var (
		store             storage.Storage
		concurrencyClient concurrency.Client
		queueClient       queue.Queue
	)
	switch cfg.Storage.Type {
	case config.StorageTypeMemory:
		store = storage.NewMemoryStorage(cfg)
	case config.StorageTypeEtcd:
		// parse connection config
		var etcdConfig config.EtcdStorageConfig
		err := mapstructure.Decode(cfg.Storage.Config, &etcdConfig)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to load etcd endpoints")
		}

		// connect to etcd
		cli, err := clientv3.New(clientv3.Config{
			Endpoints: etcdConfig.Endpoints,
			Context:   ctx,
		})
		if err != nil {
			log.Fatal().Interface("endpoints", etcdConfig.Endpoints).Msg("failed to connect to etcd")
		}

		// init storage
		s, err := storage.NewEtcdStorage(ctx, cli, "/deadman-switch/store")
		if err != nil {
			log.Fatal().Err(err).Msg("failed to connect to etcd")
		}
		// make local service configs globally available
		for _, svc := range cfg.Services {
			err := s.SaveServiceConfig(ctx, svc)
			if err != nil {
				log.Fatal().Err(err).Msg("failed to save local configs to etcd")
			}
		}
		store = s

		// setup concurrency client
		concurrencyClient, err = concurrency.NewEtcdClient(ctx, cli)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to setup concurrency client")
		}

		// setup queue client
		queueClient, err = queue.NewEtcdQueue(ctx, cli, "/deadman-switch/queue")
		if err != nil {
			log.Fatal().Err(err).Msg("failed to setup queue client")
		}
	default:
		log.Fatal().Msg("unknown storage type configured")
	}

	notifier := notifier.NewNotifier(ctx, store, queueClient)
	_ = notifier

	// setup checker which will check for deadlines and send out notifications if needed
	checker := checker.NewChecker(store, concurrencyClient, notifier, time.Duration(cfg.CheckInterval))
	log.Info().Str("backend", string(cfg.Storage.Type)).Msg("start checking deadlines")
	go checker.Backend(ctx)

	// setup server for the HTTP API (including admin endpoints and the ping endpoint)
	srv, err := server.New(ctx, cfg.HTTPListenAddress, cfg.Username, cfg.Password, store, notifier)
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("failed to initialize server")
	}
	log.Info().Str("address", cfg.HTTPListenAddress).Msg("start listening for service heatbeats")
	err = srv.Listen(ctx)
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("server stopped unexpectedly")
	}

}

func loadConfig() (cfg config.ServerConfig, err error) {
	bs, err := ioutil.ReadFile(*configFile)
	if err != nil {
		return cfg, err
	}
	err = yaml.Unmarshal(bs, &cfg)
	if err != nil {
		return cfg, err
	}
	return cfg, nil
}
