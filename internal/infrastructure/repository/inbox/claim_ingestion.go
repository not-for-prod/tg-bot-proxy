package inboxrepository

import (
	"context"
	"errors"

	"tg-bot-proxy/internal/domain/repository"
	"tg-bot-proxy/internal/infrastructure/repository/inbox/query"

	"github.com/jackc/pgx/v5"
	"github.com/not-for-prod/observer/tracer/prospan"
	proterrors "github.com/not-for-prod/proterror"
)

func (i *Implementation) ClaimIngestion(
	ctx context.Context,
	req repository.ClaimLeaseRequest,
) (*repository.Lease, bool, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	lease := &repository.Lease{BotID: req.BotID, Owner: req.Owner}

	if err := i.tr(ctx).QueryRow(ctx, query.ClaimIngestion, req.BotID, req.Owner, req.LeaseUntil).
		Scan(&lease.Offset, &lease.Version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}

		return nil, false, span.Err(proterrors.FromPG(err))
	}

	return lease, true, nil
}
