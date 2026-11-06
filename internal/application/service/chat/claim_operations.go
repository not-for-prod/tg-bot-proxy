package chat

import (
	"context"
	"time"

	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/domain/repository"

	"github.com/google/uuid"
	"github.com/not-for-prod/observer/tracer/prospan"
)

func (i *Implementation) ClaimOperations(ctx context.Context, capacity int) ([]entity.Operation, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	limit := min(capacity, i.cfg.Outbox.BatchSize, i.cfg.Outbox.WorkerPoolSize)

	if limit <= 0 {
		return nil, nil
	}

	now := time.Now().UTC()

	operations, err := i.outbox.ClaimOperations(ctx, repository.ClaimOperationsRequest{
		Limit:      limit,
		Owner:      uuid.New(),
		Now:        now,
		LeaseUntil: now.Add(i.cfg.Outbox.LeaseDuration),
	})
	if err != nil {
		return nil, span.Err(err)
	}

	return operations, nil
}
