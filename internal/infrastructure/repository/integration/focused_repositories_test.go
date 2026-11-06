package repositoryintegration_test

import (
	"tg-bot-proxy/internal/domain/repository"
	chatrepository "tg-bot-proxy/internal/infrastructure/repository/chat"
	inboxrepository "tg-bot-proxy/internal/infrastructure/repository/inbox"
	outboxrepository "tg-bot-proxy/internal/infrastructure/repository/outbox"

	txpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Implementation is a test-only facade for exercising all focused repositories
// against the same integration database. Production code injects each port separately.
type Implementation struct {
	repository.Inbox
	repository.Chat
	repository.Outbox
}

func NewImplementation(db *pgxpool.Pool, ctxGetter *txpgx.CtxGetter) *Implementation {
	return &Implementation{
		Inbox:  inboxrepository.NewImplementation(db, ctxGetter),
		Chat:   chatrepository.NewImplementation(db, ctxGetter),
		Outbox: outboxrepository.NewImplementation(db, ctxGetter),
	}
}
