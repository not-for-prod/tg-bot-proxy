package chatrepository

import (
	"context"
	"errors"

	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/domain/repository"

	txpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	proterrors "github.com/not-for-prod/proterror"
	"github.com/not-for-prod/proterror/proterror"
)

var _ repository.Chat = &Implementation{}

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

func scanChat(row pgx.Row) (*entity.Chat, error) {
	var id, externalID, supergroupID, nextOffset, committedOffset int64
	var topicID *int64

	if err := row.Scan(&id, &externalID, &supergroupID, &topicID, &nextOffset, &committedOffset); err != nil {
		return nil, proterrors.FromPG(err)
	}

	var topic *entity.TopicID

	if topicID != nil {
		value := entity.TopicID(*topicID)
		topic = &value
	}

	chat, err := entity.RehydrateChat(
		entity.ChatID(id),
		entity.ExternalChatID(externalID),
		supergroupID,
		topic,
		nextOffset,
		committedOffset,
	)
	if err != nil {
		return nil, errors.Join(&proterror.Internal{}, err)
	}

	return chat, nil
}
