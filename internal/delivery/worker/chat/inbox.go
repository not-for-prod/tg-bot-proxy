package chat

import (
	"context"

	"tg-bot-proxy/config"
	"tg-bot-proxy/internal/domain/service"
	sharedworker "tg-bot-proxy/internal/pkg/worker"
)

func NewInboxProcessor(
	proxy service.TelegramProxy,
	metrics *sharedworker.Metrics,
) sharedworker.Worker {
	cfg := config.Instance().Inbox

	return sharedworker.New(
		sharedworker.Config{Name: "telegram-inbox", Interval: cfg.Interval},
		func(ctx context.Context) error {
			return proxy.ProcessInboxBatch(ctx)
		},
		metrics,
	)
}
