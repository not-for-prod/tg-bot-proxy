package chat

import (
	"math"
	"time"

	"tg-bot-proxy/config"
	"tg-bot-proxy/internal/domain/gateway"
	"tg-bot-proxy/internal/domain/repository"
	"tg-bot-proxy/internal/domain/service"

	"github.com/avito-tech/go-transaction-manager/trm/v2"
)

var _ service.TelegramProxy = &Implementation{}

type Implementation struct {
	cfg      *config.Config
	inbox    repository.Inbox
	chats    repository.Chat
	outbox   repository.Outbox
	telegram gateway.Telegram
	tx       trm.Manager
	metrics  *Metrics
}

type Dependencies struct {
	Inbox    repository.Inbox
	Chats    repository.Chat
	Outbox   repository.Outbox
	Telegram gateway.Telegram
	Tx       trm.Manager
	Metrics  *Metrics
}

func NewImplementation(dependencies Dependencies) *Implementation {
	return &Implementation{
		cfg:      config.Instance(),
		inbox:    dependencies.Inbox,
		chats:    dependencies.Chats,
		outbox:   dependencies.Outbox,
		telegram: dependencies.Telegram,
		tx:       dependencies.Tx,
		metrics:  dependencies.Metrics,
	}
}

func retryAt(policy config.Retry, attempt int, now time.Time) time.Time {
	delay := float64(policy.InitialDelay) * math.Pow(policy.Multiplier, float64(max(attempt-1, 0)))

	if delay > float64(policy.MaxDelay) {
		delay = float64(policy.MaxDelay)
	}

	return now.Add(time.Duration(delay))
}
