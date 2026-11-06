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

func (i *Implementation) FailOperation(ctx context.Context, op entity.Operation, failure repository.Failure) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	query, args, err := sq.Update(xo.Table_Outbox).
		Set(xo.Field_Outbox_Status, entity.OperationFailed).
		Set(xo.Field_Outbox_LeaseOwner, nil).
		Set(xo.Field_Outbox_LeaseUntil, nil).
		Set(xo.Field_Outbox_LastHTTPStatus, failure.HTTPStatus).
		Set(xo.Field_Outbox_LastErrorCode, failure.ErrorCode).
		Set(xo.Field_Outbox_LastError, failure.Message).
		Set(xo.Field_Outbox_RetryAfterSeconds, failure.RetryAfterSeconds).
		Set(xo.Field_Outbox_FailedAt, sq.Expr("clock_timestamp()")).
		Set(xo.Field_Outbox_UpdatedAt, sq.Expr("clock_timestamp()")).
		Where(sq.Eq{
			xo.Field_Outbox_ID:           op.ID,
			xo.Field_Outbox_LeaseOwner:   op.LeaseOwner,
			xo.Field_Outbox_LeaseVersion: op.LeaseVersion,
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
