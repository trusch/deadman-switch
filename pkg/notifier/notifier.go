package notifier

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"
	"github.com/trusch/deadman-switch/pkg/config"
	"github.com/trusch/deadman-switch/pkg/queue"
	"github.com/trusch/deadman-switch/pkg/storage"
)

type Notifier interface {
	SendAlerts(ctx context.Context, service config.ServiceConfig) error
	SendRecoveryNotifications(ctx context.Context, service config.ServiceConfig) error
}

func NewNotifier(ctx context.Context, store storage.Storage, queue queue.Queue) Notifier {
	notifier := &defaultNotifierType{
		store: store,
		queue: queue,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
	if notifier.queue != nil {
		go func() {
			err := notifier.getAndProcessNotificationsFromQueue(ctx)
			if err != nil {
				log.Error().Err(err).Msg("stopped reading notification tasks from queue")
			}
		}()
	}

	return notifier
}

type defaultNotifierType struct {
	queue      queue.Queue
	store      storage.Storage
	httpClient *http.Client
}

func (n *defaultNotifierType) SendAlerts(ctx context.Context, service config.ServiceConfig) (err error) {
	if service.Debounce > 0 {
		lastMessageSend, err := n.store.GetLastMessageSendTimestamp(ctx, service.ID)
		if err == nil {
			if time.Now().Add(-time.Duration(service.Debounce)).Before(lastMessageSend) {
				log.Info().Str("service", service.ID).Msg("don't enqueue alert messages because of debouncing")
				return nil
			}
		}
	}

	log.Info().Str("service", service.ID).Msg("send out alert messages")
	for _, notification := range service.AlertNotifications {
		if n.queue != nil {
			log.Debug().
				Str("service", service.ID).
				Msg("enqueuing notification call")
			err = n.queue.Enqueue(ctx, notificationWrapper{
				Service:      service,
				Notification: notification,
			})
			if err != nil {
				return err
			}
		} else {
			// no queue, direct calling
			switch notification.Type {
			case config.NotificationTypeWebhook:
				cfg, err := notification.GetWebhookConfig()
				if err != nil {
					return err
				}
				err = n.sendAlertToWebhook(ctx, service, cfg)
			case config.NotificationTypeSlack:
				cfg, err := notification.GetSlackConfig()
				if err != nil {
					return err
				}
				err = n.sendAlertToSlack(ctx, service, cfg)
			default:
				return errors.New("unimplemented notification type")
			}
			if err != nil {
				return err
			}
		}
	}

	err = n.store.SetLastMessageSendTimestamp(ctx, service.ID, time.Now())
	if err != nil {
		return err
	}

	return nil
}

func (n *defaultNotifierType) SendRecoveryNotifications(ctx context.Context, service config.ServiceConfig) (err error) {
	log.Info().Str("service", service.ID).Msg("send out recovery messages")
	for _, notification := range service.RecoveryNotifications {
		if n.queue != nil {
			log.Debug().
				Str("service", service.ID).
				Msg("enqueuing notification call")
			err = n.queue.Enqueue(ctx, notificationWrapper{
				Service:           service,
				Notification:      notification,
				IsRecoveryMessage: true,
			})
			if err != nil {
				return err
			}
		} else {
			// no queue, direct calling
			switch notification.Type {
			case config.NotificationTypeWebhook:
				cfg, err := notification.GetWebhookConfig()
				if err != nil {
					return err
				}
				err = n.sendRecoveryToWebhook(ctx, service, cfg)
			case config.NotificationTypeSlack:
				cfg, err := notification.GetSlackConfig()
				if err != nil {
					return err
				}
				err = n.sendRecoveryToSlack(ctx, service, cfg)
			default:
				return errors.New("unimplemented notification type")
			}
			if err != nil {
				return err
			}
		}
	}
	err = n.store.SetLastMessageSendTimestamp(ctx, service.ID, time.Now())
	if err != nil {
		return err
	}

	return nil
}

func (n *defaultNotifierType) sendAlertToWebhook(ctx context.Context, service config.ServiceConfig, cfg config.WebhookConfig) error {
	log.Info().
		Str("service", service.ID).
		Str("method", cfg.Method).
		Str("url", cfg.URL).
		Msg("calling webhook")
	r, _ := http.NewRequest(cfg.Method, cfg.URL, strings.NewReader(cfg.Body))
	r = r.WithContext(ctx)
	if cfg.Headers != nil {
		r.Header = cfg.Headers
	}
	_, err := n.httpClient.Do(r)
	if err != nil {
		return err
	}

	return err
}

func (n *defaultNotifierType) sendAlertToSlack(ctx context.Context, service config.ServiceConfig, cfg config.SlackConfig) error {
	log.Info().
		Str("service", service.ID).
		Str("channel", cfg.Channel).
		Msg("sending slack message")

	attachment := slack.Attachment{
		Title: "ALERT",
		Color: "danger",
		Text:  fmt.Sprintf("The service %s has stopped sending heartbeats", service.ID),
		Fields: []slack.AttachmentField{
			slack.AttachmentField{
				Title: "service",
				Value: service.ID,
			},
		},
	}

	lastHearbeat, err := n.store.GetLastHeartbeat(ctx, service.ID)
	if err == nil {
		attachment.Fields = append(attachment.Fields, slack.AttachmentField{
			Title: "last heartbeat",
			Value: fmt.Sprintf("%s", lastHearbeat.Format(time.RFC3339)),
		})
	} else {
		log.Error().Str("service", service.ID).Err(err).Msg("can't load last heartbeat")
	}
	for _, field := range cfg.MessageFields {
		attachment.Fields = append(attachment.Fields, slack.AttachmentField{
			Title: field.Key,
			Value: field.Value,
		})
	}

	api := slack.New(cfg.Token)
	_, _, err = api.PostMessage(
		cfg.Channel,
		slack.MsgOptionAsUser(true),
		slack.MsgOptionAttachments(attachment),
	)
	if err != nil {
		return err
	}

	return nil
}

func (n *defaultNotifierType) sendRecoveryToWebhook(ctx context.Context, service config.ServiceConfig, cfg config.WebhookConfig) error {
	log.Info().
		Str("service", service.ID).
		Str("method", cfg.Method).
		Str("url", cfg.URL).
		Msg("calling webhook")
	r, _ := http.NewRequest(cfg.Method, cfg.URL, strings.NewReader(cfg.Body))
	r = r.WithContext(ctx)
	if cfg.Headers != nil {
		r.Header = cfg.Headers
	}
	_, err := n.httpClient.Do(r)
	if err != nil {
		return err
	}

	return err
}

func (n *defaultNotifierType) sendRecoveryToSlack(ctx context.Context, service config.ServiceConfig, cfg config.SlackConfig) error {
	log.Info().
		Str("service", service.ID).
		Str("channel", cfg.Channel).
		Msg("sending slack message")

	attachment := slack.Attachment{
		Title: "RECOVERY",
		Color: "good",
		Text:  fmt.Sprintf("The service %s started sending heartbeats again", service.ID),
		Fields: []slack.AttachmentField{
			slack.AttachmentField{
				Title: "service",
				Value: service.ID,
			},
		},
	}

	lastHearbeat, err := n.store.GetLastHeartbeat(ctx, service.ID)
	if err == nil {
		attachment.Fields = append(attachment.Fields, slack.AttachmentField{
			Title: "last heartbeat",
			Value: fmt.Sprintf("%s", lastHearbeat.Format(time.RFC3339)),
		})
	} else {
		log.Error().Str("service", service.ID).Err(err).Msg("can't load last heartbeat")
	}
	for _, field := range cfg.MessageFields {
		attachment.Fields = append(attachment.Fields, slack.AttachmentField{
			Title: field.Key,
			Value: field.Value,
		})
	}

	api := slack.New(cfg.Token)
	_, _, err = api.PostMessage(
		cfg.Channel,
		slack.MsgOptionAsUser(true),
		slack.MsgOptionAttachments(attachment),
	)
	if err != nil {
		return err
	}

	return nil
}

func (n *defaultNotifierType) getAndProcessNotificationsFromQueue(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			var task notificationWrapper
			err := n.queue.Dequeue(ctx, &task)
			if err != nil {
				return err
			}
			switch task.Notification.Type {
			case config.NotificationTypeWebhook:
				cfg, err := task.Notification.GetWebhookConfig()
				if err != nil {
					return err
				}
				if task.IsRecoveryMessage {
					err = n.sendRecoveryToWebhook(ctx, task.Service, cfg)
				} else {
					err = n.sendAlertToWebhook(ctx, task.Service, cfg)
				}
			case config.NotificationTypeSlack:
				cfg, err := task.Notification.GetSlackConfig()
				if err != nil {
					return err
				}
				if task.IsRecoveryMessage {
					err = n.sendRecoveryToSlack(ctx, task.Service, cfg)
				} else {
					err = n.sendAlertToSlack(ctx, task.Service, cfg)
				}
			default:
				return errors.New("unimplemented notification type")
			}
			if err != nil {
				return err
			}
		}
	}
}

type notificationWrapper struct {
	Service           config.ServiceConfig      `json:"service"`
	Notification      config.NotificationConfig `json:"notification"`
	IsRecoveryMessage bool                      `json:"isRecoveryMessage"`
}
