package outboxrepository

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

func (i *Implementation) CompleteOperation(ctx context.Context, op entity.Operation) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	deleteQuery, deleteArgs, err := sq.Delete(xo.Table_Outbox).
		Where(sq.Eq{
			xo.Field_Outbox_ID:           op.ID,
			xo.Field_Outbox_LeaseOwner:   op.LeaseOwner,
			xo.Field_Outbox_LeaseVersion: op.LeaseVersion,
			xo.Field_Outbox_Status:       entity.OperationProcessing,
		}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return span.Err(errors.Join(&proterror.Internal{}, err))
	}

	result, err := i.tr(ctx).Exec(ctx, deleteQuery, deleteArgs...)
	if err != nil {
		return span.Err(proterrors.FromPG(err))
	}

	if result.RowsAffected() != 1 {
		return span.Err(errors.Join(&proterror.FailedPrecondition{}, repository.ErrStaleLease))
	}

	return nil
}
