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

func (i *Implementation) DeleteInbox(ctx context.Context, lease repository.Lease, id int64) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	leaseExists := inboxProcessorLeaseExists(lease)

	query, args, err := sq.Delete(xo.Table_Inbox_With_Alias).
		Where(sq.Eq{
			xo.Field_Inbox_ID_With_Alias:    id,
			xo.Field_Inbox_BotID_With_Alias: lease.BotID,
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
