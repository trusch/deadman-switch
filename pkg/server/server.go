package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/rs/zerolog/log"
	"github.com/trusch/deadman-switch/pkg/config"
	"github.com/trusch/deadman-switch/pkg/notifier"
	"github.com/trusch/deadman-switch/pkg/storage"
)

type Server struct {
	listenAddress      string
	username, password string
	mutex              sync.RWMutex
	lastHeartbeats     map[string]time.Time
	cli                *http.Client
	store              storage.Storage
	notifier           notifier.Notifier
}

func New(ctx context.Context, listenAddress, username, password string, store storage.Storage, notifier notifier.Notifier) (*Server, error) {
	srv := &Server{
		listenAddress:  listenAddress,
		username:       username,
		password:       password,
		lastHeartbeats: make(map[string]time.Time),
		cli: &http.Client{
			Timeout: 5 * time.Second,
		},
		store:    store,
		notifier: notifier,
	}

	return srv, nil
}

func (s *Server) Listen(ctx context.Context) (err error) {
	router := chi.NewRouter()
	router.HandleFunc("/ping/{serviceID}", s.handlePing)
	router.Route("/config", func(r chi.Router) {
		r.Use(middleware.BasicAuth("deadman-switch", map[string]string{
			s.username: s.password,
		}))
		r.Get("/", s.handleListConfigs)
		r.Post("/", s.handleCreateConfig)
		r.Delete("/{serviceID}", s.handleDeleteConfig)
	})

	srv := &http.Server{
		Addr:    s.listenAddress,
		Handler: router,
	}

	go func() {
		err = srv.ListenAndServe()
		if err != nil {
			log.Error().Err(err).Msg("failed to listen")
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shutdownErr := srv.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		log.Error().Err(shutdownErr).Msg("failed to shutdown the server")
	}

	return err
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "serviceID")
	svcConfig, err := s.store.GetServiceConfig(r.Context(), serviceID)
	if err != nil {
		log.Error().Str("service", serviceID).Err(err).Msg("failed to load service config")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("nice to meet you stranger"))
		return
	}
	if svcConfig.Token != "" {
		if r.URL.Query().Get("token") != svcConfig.Token {
			log.Warn().Str("service", serviceID).Msg("failed to validate token")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("you might wish to supply a correct token for this request"))
			return
		}
	}
	log.Info().Str("service", serviceID).Msg("received heartbeat")
	s.updateLastHeartbeat(r.Context(), svcConfig)
	w.Write([]byte(fmt.Sprintf("got it %s, you are still alive", serviceID)))
}

func (s *Server) handleDeleteConfig(w http.ResponseWriter, r *http.Request) {
	service := chi.URLParam(r, "serviceID")
	err := s.store.DeleteServiceConfig(r.Context(), service)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
}

func (s *Server) handleListConfigs(w http.ResponseWriter, r *http.Request) {
	var configs []config.ServiceConfig
	configChan, errChan := s.store.GetServiceConfigs(r.Context())
loop:
	for {
		select {
		case <-r.Context().Done():
			break loop
		case cfg, ok := <-configChan:
			if !ok {
				break loop
			}
			configs = append(configs, cfg)
		case err := <-errChan:
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				log.Error().Err(err).Msg("failed to list service configs")
				return
			}
		}
	}
	err := json.NewEncoder(w).Encode(configs)
	if err != nil {
		log.Error().Err(err).Msg("failed encode and send configs")
	}
}

func (s *Server) handleCreateConfig(w http.ResponseWriter, r *http.Request) {
	var cfg config.ServiceConfig
	defer r.Body.Close()
	err := json.NewDecoder(r.Body).Decode(&cfg)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		log.Error().Err(err).Msg("failed to decode service config")
		return
	}
	err = s.store.SaveServiceConfig(r.Context(), cfg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Error().Err(err).Msg("failed to save new service config")
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) updateLastHeartbeat(ctx context.Context, svc config.ServiceConfig) {
	err := s.store.SetLastHeartbeat(ctx, svc.ID, time.Now())
	if err != nil {
		log.Error().Str("service", svc.ID).Err(err).Msg("failed to update timestamp")
	}
	_, err = s.store.GetAlarmActiveSince(ctx, svc.ID)
	if err == nil {
		err = s.store.ClearAlarm(ctx, svc.ID)
		if err != nil {
			log.Error().Str("service", svc.ID).Err(err).Msg("failed to clear alarm timestamp")
		}
		err = s.notifier.SendRecoveryNotifications(ctx, svc)
		if err != nil {
			log.Error().Str("service", svc.ID).Err(err).Msg("failed to send recovery notifications")
		}
	}
}
