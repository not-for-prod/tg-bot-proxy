package inboxrepository

import (
	"context"
	"errors"
	"fmt"

	"tg-bot-proxy/internal/domain/repository"
	"tg-bot-proxy/internal/generated/xo"

	sq "github.com/Masterminds/squirrel"
	"github.com/not-for-prod/observer/tracer/prospan"
	proterrors "github.com/not-for-prod/proterror"
	"github.com/not-for-prod/proterror/proterror"
)

func (i *Implementation) CommitIngestion(
	ctx context.Context,
	req repository.CommitIngestionRequest,
) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	offsetQuery, offsetArgs, err := sq.Update(xo.Table_TelegramBotOffset).
		Set(
			xo.Field_TelegramBotOffset_UpdateID,
			sq.Expr(fmt.Sprintf("GREATEST(%s, ?)", xo.Field_TelegramBotOffset_UpdateID), req.NextOffset),
		).
		Set(xo.Field_TelegramBotOffset_LeaseOwner, nil).
		Set(xo.Field_TelegramBotOffset_LeaseUntil, nil).
		Where(
			sq.Eq{
				xo.Field_TelegramBotOffset_BotID:        req.Lease.BotID,
				xo.Field_TelegramBotOffset_LeaseOwner:   req.Lease.Owner,
				xo.Field_TelegramBotOffset_LeaseVersion: req.Lease.Version,
			},
		).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return span.Err(errors.Join(&proterror.Internal{}, err))
	}

	result, err := i.tr(ctx).Exec(ctx, offsetQuery, offsetArgs...)
	if err != nil {
		return span.Err(proterrors.FromPG(err))
	}

	if result.RowsAffected() != 1 {
		return span.Err(errors.Join(&proterror.FailedPrecondition{}, repository.ErrStaleLease))
	}

	return nil
}
