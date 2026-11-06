package repository

import (
	"context"

	"tg-bot-proxy/internal/domain/entity"
)

type Chat interface {
	GetChatByExternalID(context.Context, entity.ExternalChatID) (*entity.Chat, error)
	GetChatByTopic(context.Context, int64, entity.TopicID) (*entity.Chat, error)
	CreateChat(context.Context, *entity.Chat) error
	UpdateChat(context.Context, *entity.Chat) error
	CommitChatOffset(context.Context, entity.ChatID, int64) error
}
