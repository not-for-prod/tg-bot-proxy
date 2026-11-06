package outboxrepository

import (
	"context"

	"tg-bot-proxy/internal/domain/repository"

	txpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ repository.Outbox = &Implementation{}

type Implementation struct {
	db        *pgxpool.Pool
	ctxGetter *txpgx.CtxGetter
}

func NewImplementation(db *pgxpool.Pool, ctxGetter *txpgx.CtxGetter) *Implementation {
	return &Implementation{db: db, ctxGetter: ctxGetter}
}

func (i *Implementation) tr(ctx context.Context) txpgx.Tr {
	return i.ctxGetter.DefaultTrOrDB(ctx, i.db)
}
