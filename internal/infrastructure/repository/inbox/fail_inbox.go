package inboxrepository

import (
	"context"
	"errors"

	"tg-bot-proxy/internal/domain/repository"
	"tg-bot-proxy/internal/generated/xo"

	sq "github.com/Masterminds/squirrel"
	"github.com/not-for-prod/observer/tracer/prospan"
	proterrors "github.com/not-for-prod/proterror"
	"github.com/not-for-prod/proterror/proterror"
)

func (i *Implementation) FailInbox(
	ctx context.Context,
	req repository.FailInboxRequest,
) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	leaseExists := inboxProcessorLeaseExists(req.Lease)

	query, args, err := sq.Update(xo.Table_Inbox_With_Alias).
		Set(xo.Field_Inbox_Status, "failed").
		Set(xo.Field_Inbox_AttemptCount, sq.Expr(xo.Field_Inbox_AttemptCount+" + 1")).
		Set(xo.Field_Inbox_LastError, req.Message).
		Set(xo.Field_Inbox_FailedAt, sq.Expr("clock_timestamp()")).
		Set(xo.Field_Inbox_UpdatedAt, sq.Expr("clock_timestamp()")).
		Where(sq.Eq{
			xo.Field_Inbox_ID_With_Alias:    req.Item.ID,
			xo.Field_Inbox_BotID_With_Alias: req.Lease.BotID,
		}).
		Where(sq.Expr("EXISTS (?)", leaseExists)).
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
