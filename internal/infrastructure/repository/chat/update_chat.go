package chatrepository

import (
	"context"
	"errors"

	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/generated/xo"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/not-for-prod/observer/tracer/prospan"
	proterrors "github.com/not-for-prod/proterror"
	"github.com/not-for-prod/proterror/proterror"
)

func (i *Implementation) UpdateChat(ctx context.Context, chat *entity.Chat) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	query, args, err := sq.Update(xo.Table_Chat).
		Set(xo.Field_Chat_TopicID, chat.TopicID).
		Set(xo.Field_Chat_NextChatOffset, chat.NextChatOffset).
		Set(xo.Field_Chat_CommittedChatOffset, chat.CommittedChatOffset).
		Set(xo.Field_Chat_LastActivityAt, sq.Expr("clock_timestamp()")).
		Where(sq.Eq{xo.Field_Chat_ID: chat.ID}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return span.Err(errors.Join(&proterror.Internal{}, err))
	}

	result, err := i.tr(ctx).Exec(ctx, query, args...)
	if err != nil {
		return span.Err(proterrors.FromPG(err))
	}

	if result.RowsAffected() != 1 {
		return span.Err(proterrors.FromPG(pgx.ErrNoRows))
	}

	return nil
}
