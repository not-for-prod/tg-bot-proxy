package chatrepository

import (
	"context"
	"errors"

	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/domain/repository"
	"tg-bot-proxy/internal/generated/xo"

	sq "github.com/Masterminds/squirrel"
	"github.com/not-for-prod/observer/tracer/prospan"
	proterrors "github.com/not-for-prod/proterror"
	"github.com/not-for-prod/proterror/proterror"
)

func (i *Implementation) CommitChatOffset(ctx context.Context, chatID entity.ChatID, offset int64) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	query, args, err := sq.Update(xo.Table_Chat).
		Set(xo.Field_Chat_CommittedChatOffset, offset).
		Set(xo.Field_Chat_LastActivityAt, sq.Expr("clock_timestamp()")).
		Where(sq.Eq{
			xo.Field_Chat_ID:                  chatID,
			xo.Field_Chat_CommittedChatOffset: offset - 1,
		}).
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
		return span.Err(errors.Join(&proterror.FailedPrecondition{}, repository.ErrStaleLease))
	}

	return nil
}
