package chat

import (
	"context"

	"tg-bot-proxy/config"
	"tg-bot-proxy/internal/domain/service"
	sharedworker "tg-bot-proxy/internal/pkg/worker"
)

func NewPoller(
	proxy service.TelegramProxy,
	metrics *sharedworker.Metrics,
) sharedworker.Worker {
	cfg := config.Instance().Telegram.Bot

	return sharedworker.New(
		sharedworker.Config{Name: "telegram-poller", Interval: cfg.Polling.Interval},
		func(ctx context.Context) error {
			return proxy.Poll(ctx)
		},
		metrics,
	)
}
