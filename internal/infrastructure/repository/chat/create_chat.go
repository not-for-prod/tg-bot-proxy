package chatrepository

import (
	"context"
	"errors"

	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/generated/xo"

	sq "github.com/Masterminds/squirrel"
	"github.com/not-for-prod/observer/tracer/prospan"
	proterrors "github.com/not-for-prod/proterror"
	"github.com/not-for-prod/proterror/proterror"
)

func (i *Implementation) CreateChat(ctx context.Context, chat *entity.Chat) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	query, args, err := sq.Insert(xo.Table_Chat).
		Columns(
			xo.Field_Chat_ExternalChatID,
			xo.Field_Chat_SupergroupID,
			xo.Field_Chat_TopicID,
			xo.Field_Chat_NextChatOffset,
			xo.Field_Chat_CommittedChatOffset,
		).
		Values(chat.ExternalChatID, chat.SupergroupID, chat.TopicID, chat.NextChatOffset, chat.CommittedChatOffset).
		Suffix("RETURNING " + xo.Field_Chat_ID).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return span.Err(errors.Join(&proterror.Internal{}, err))
	}

	if err = i.tr(ctx).QueryRow(ctx, query, args...).Scan(&chat.ID); err != nil {
		return span.Err(proterrors.FromPG(err))
	}

	return nil
}
