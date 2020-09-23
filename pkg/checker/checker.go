package checker

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/trusch/deadman-switch/pkg/concurrency"
	"github.com/trusch/deadman-switch/pkg/config"
	"github.com/trusch/deadman-switch/pkg/notifier"
	"github.com/trusch/deadman-switch/pkg/storage"
)

type Checker struct {
	store       storage.Storage
	concurrency concurrency.Client
	notifier    notifier.Notifier
	interval    time.Duration
	cli         *http.Client
}

func NewChecker(
	store storage.Storage,
	concurrency concurrency.Client,
	notifier notifier.Notifier,
	interval time.Duration,
) *Checker {
	return &Checker{store, concurrency, notifier, interval, &http.Client{Timeout: 5 * time.Second}}
}

func (c *Checker) Backend(ctx context.Context) error {
	wg := &sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := c.checkDeadlinesIfLeader(ctx)
				if err != nil {
					log.Error().Err(err).Msg("error while checking deadlines")
				}
			}
		}
	}()

	wg.Wait()
	return ctx.Err()
}

func (c *Checker) checkDeadlinesIfLeader(ctx context.Context) error {
	if c.concurrency != nil {
		isLeader, err := c.concurrency.IsLeader(ctx, "/deadman-switch/check-leader")
		if err != nil {
			if err == context.DeadlineExceeded {
				return nil
			}
			return err
		}
		if !isLeader {
			return nil
		}
		return c.checkDeadlines(ctx)
	}
	return c.checkDeadlines(ctx)
}

func (c *Checker) checkDeadlines(ctx context.Context) error {
	configs, errorChannel := c.store.GetServiceConfigs(ctx)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errorChannel:
			if err != nil {
				log.Error().Err(err).Msg("error reading service configs")
			}
		case svc, ok := <-configs:
			if !ok {
				return nil
			}
			err := c.checkDeadlineOfService(ctx, svc)
			if err != nil {
				log.Error().Str("service", svc.ID).Err(err).Msg("failed to check deadline")
			}
		}
	}
}

func (c *Checker) checkDeadlineOfService(ctx context.Context, svc config.ServiceConfig) error {
	t, err := c.store.GetLastHeartbeat(ctx, svc.ID)
	if err != nil {
		log.Error().Str("service", svc.ID).Err(err).Msg("failed to get last heartbeat")
	}
	timeSinceLastHeartbeat := time.Since(t)
	if timeSinceLastHeartbeat > time.Duration(svc.Timeout) {
		log.Info().Str("service", svc.ID).Msg("service is overdue")
		_, err := c.store.GetAlarmActiveSince(ctx, svc.ID)
		if err == storage.ErrNotFound {
			err = c.store.SetAlarmActiveSince(ctx, svc.ID, time.Now())
			if err != nil {
				log.Error().Str("service", svc.ID).Err(err).Msg("failed to set alarm active state")
			}
		}
		err = c.notifier.SendAlerts(ctx, svc)
		if err != nil {
			return err
		}
	} else {
		log.Info().
			Str("service", svc.ID).
			Time("last_heartbeat", time.Now().Add(-timeSinceLastHeartbeat)).
			Msg("service is considered alive")
	}
	return nil
}
