package service

import (
	"context"

	"tg-bot-proxy/internal/domain/entity"
)

type TelegramProxy interface {
	Poll(context.Context) error
	ProcessInboxBatch(context.Context) error
	ClaimOperations(context.Context, int) ([]entity.Operation, error)
	ExecuteOperation(context.Context, entity.Operation) error
}
