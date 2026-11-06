package chatrepository

import (
	"context"
	"errors"

	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/generated/xo"

	sq "github.com/Masterminds/squirrel"
	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"
)

func (i *Implementation) GetChatByExternalID(
	ctx context.Context,
	id entity.ExternalChatID,
) (*entity.Chat, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	query := sq.Select(
		xo.Field_Chat_ID,
		xo.Field_Chat_ExternalChatID,
		xo.Field_Chat_SupergroupID,
		xo.Field_Chat_TopicID,
		xo.Field_Chat_NextChatOffset,
		xo.Field_Chat_CommittedChatOffset,
	).
		From(xo.Table_Chat).
		Where(sq.Eq{xo.Field_Chat_ExternalChatID: id}).
		PlaceholderFormat(sq.Dollar).
		Suffix("FOR UPDATE")

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, span.Err(errors.Join(&proterror.Internal{}, err))
	}

	chat, err := scanChat(i.tr(ctx).QueryRow(ctx, sql, args...))
	if err != nil {
		return nil, span.Err(err)
	}

	return chat, nil
}
