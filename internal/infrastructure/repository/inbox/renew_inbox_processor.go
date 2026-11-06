package inboxrepository

import (
	"context"
	"errors"
	"time"

	"tg-bot-proxy/internal/domain/repository"
	"tg-bot-proxy/internal/generated/xo"

	sq "github.com/Masterminds/squirrel"
	"github.com/not-for-prod/observer/tracer/prospan"
	proterrors "github.com/not-for-prod/proterror"
	"github.com/not-for-prod/proterror/proterror"
)

func (i *Implementation) RenewInboxProcessor(ctx context.Context, lease repository.Lease, until time.Time) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	query, args, err := sq.Update(xo.Table_InboxProcessorLock).
		Set(xo.Field_InboxProcessorLock_LeaseUntil, until).
		Where(sq.Eq{
			xo.Field_InboxProcessorLock_BotID:        lease.BotID,
			xo.Field_InboxProcessorLock_LeaseOwner:   lease.Owner,
			xo.Field_InboxProcessorLock_LeaseVersion: lease.Version,
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
