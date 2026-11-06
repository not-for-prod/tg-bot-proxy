package outboxrepository

import (
	"context"
	"errors"
	"time"

	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/domain/repository"
	"tg-bot-proxy/internal/generated/xo"

	sq "github.com/Masterminds/squirrel"
	"github.com/not-for-prod/observer/tracer/prospan"
	proterrors "github.com/not-for-prod/proterror"
	"github.com/not-for-prod/proterror/proterror"
)

func (i *Implementation) RenewOperation(ctx context.Context, operation entity.Operation, until time.Time) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	query, args, err := sq.Update(xo.Table_Outbox).
		Set(xo.Field_Outbox_LeaseUntil, until).
		Set(xo.Field_Outbox_UpdatedAt, sq.Expr("clock_timestamp()")).
		Where(sq.Eq{
			xo.Field_Outbox_ID:           operation.ID,
			xo.Field_Outbox_Status:       entity.OperationProcessing,
			xo.Field_Outbox_LeaseOwner:   operation.LeaseOwner,
			xo.Field_Outbox_LeaseVersion: operation.LeaseVersion,
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
