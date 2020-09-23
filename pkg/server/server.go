package server

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog/log"
	"github.com/trusch/deadman-switch/pkg/config"
	"github.com/trusch/deadman-switch/pkg/storage"
)

type Server struct {
	cfg            config.ServerConfig
	mutex          sync.RWMutex
	lastHeartbeats map[string]time.Time
	cli            *http.Client
	store          storage.Storage
}

func New(ctx context.Context, cfg config.ServerConfig) *Server {
	srv := &Server{
		cfg:            cfg,
		lastHeartbeats: make(map[string]time.Time),
		cli: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	switch cfg.Storage.Type {
	case config.StorageTypeMemory:
		srv.store = storage.NewMemoryStorage()
	case config.StorageTypeEtcd:
		var endpoints []string
		err := mapstructure.Decode(cfg.Storage.Config["endpoints"], &endpoints)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to load etcd endpoints")
		}
		store, err := storage.NewEtcdStorage(endpoints)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to connect to etcd")
		}
		srv.store = store
	default:
		log.Fatal().Msg("unknown storage type configured")
	}

	go func() {
		ticker := time.NewTicker(time.Duration(cfg.CheckInterval))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				srv.callWebhooks(ctx)
			}
		}
	}()

	return srv
}

func (s *Server) Listen(ctx context.Context) (err error) {
	srv := &http.Server{
		Addr:    s.cfg.HTTPListenAddress,
		Handler: s,
	}
	go func() { err = srv.ListenAndServe() }()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shutdownErr := srv.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		log.Error().Err(shutdownErr).Msg("failed to shutdown the server")
	}
	return err
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	log.Info().Str("from", id).Msg("received heartbeat")
	s.updateLastHeartbeat(r.Context(), id)
	w.Write([]byte("got it, you are still alive"))
}

func (s *Server) updateLastHeartbeat(ctx context.Context, id string) {
	err := s.store.SetTimestamp(ctx, id, time.Now())
	if err != nil {
		log.Error().Err(err).Msg("failed to update timestamp")
	}
}

func (s *Server) getLastHeartbeat(ctx context.Context, id string) time.Time {
	t, err := s.store.GetTimestamp(ctx, id)
	if err != nil {
		log.Error().Err(err).Msg("failed to get last heartbeat")
	}
	return t
}

func (s *Server) callWebhooks(ctx context.Context) {
	for _, service := range s.cfg.Services {
		timeSinceLastHeartbeat := time.Since(s.getLastHeartbeat(ctx, service.ID))
		if timeSinceLastHeartbeat > time.Duration(service.Timeout) {
			for _, webhook := range service.Webhooks {
				log.Info().
					Str("service", service.ID).
					Str("method", webhook.Method).
					Str("url", webhook.URL).
					Msg("calling webhook")
				r, _ := http.NewRequest(webhook.Method, webhook.URL, strings.NewReader(webhook.Body))
				_, err := s.cli.Do(r)
				if err != nil {
					log.Error().
						Str("service", service.ID).
						Str("method", webhook.Method).
						Str("url", webhook.URL).
						Err(err).
						Msg("failed to call webhook")
				}
			}
		} else {
			log.Info().
				Str("service", service.ID).
				Time("last_heartbeat", time.Now().Add(-timeSinceLastHeartbeat)).
				Msg("service is not overdue")
		}
	}
}
