package outboxrepository

import (
	"context"
	"errors"

	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/domain/repository"
	"tg-bot-proxy/internal/generated/xo"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/not-for-prod/observer/tracer/prospan"
	proterrors "github.com/not-for-prod/proterror"
	"github.com/not-for-prod/proterror/proterror"
)

func (i *Implementation) LockOperation(ctx context.Context, operation entity.Operation) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	query, args, err := sq.Select(xo.Field_Outbox_ID).
		From(xo.Table_Outbox).
		Where(sq.Eq{
			xo.Field_Outbox_ID:           operation.ID,
			xo.Field_Outbox_ChatID:       operation.ChatID,
			xo.Field_Outbox_Status:       entity.OperationProcessing,
			xo.Field_Outbox_LeaseOwner:   operation.LeaseOwner,
			xo.Field_Outbox_LeaseVersion: operation.LeaseVersion,
		}).
		Suffix("FOR UPDATE").
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return span.Err(errors.Join(&proterror.Internal{}, err))
	}

	var id int64

	if err = i.tr(ctx).QueryRow(ctx, query, args...).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return span.Err(errors.Join(&proterror.FailedPrecondition{}, repository.ErrStaleLease))
		}

		return span.Err(proterrors.FromPG(err))
	}

	return nil
}
