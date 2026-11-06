package chat

import (
	"context"
	"errors"
	"time"

	"tg-bot-proxy/internal/domain/gateway"
	"tg-bot-proxy/internal/domain/repository"

	"github.com/google/uuid"
	"github.com/not-for-prod/observer/tracer/prospan"
)

func (i *Implementation) Poll(ctx context.Context) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	cfg := i.cfg.Telegram.Bot

	lease, claimed, err := i.inbox.ClaimIngestion(ctx, repository.ClaimLeaseRequest{
		BotID:      cfg.ID,
		Owner:      uuid.New(),
		LeaseUntil: time.Now().UTC().Add(cfg.Polling.LeaseDuration),
	})
	if err != nil {
		return span.Err(err)
	}

	if !claimed {
		return nil
	}

	updates, err := i.telegram.GetUpdates(
		ctx,
		gateway.GetUpdatesRequest{
			Offset:         lease.Offset,
			Limit:          cfg.Polling.BatchSize,
			TimeoutSeconds: int(cfg.Polling.Timeout / time.Second),
			AllowedUpdates: cfg.Polling.AllowedUpdates,
		},
	)
	if err != nil {
		return span.Err(errors.Join(err, i.inbox.ReleaseIngestion(ctx, *lease)))
	}

	if len(updates) == 0 {
		if err = i.inbox.ReleaseIngestion(ctx, *lease); err != nil {
			return span.Err(err)
		}

		return nil
	}

	nextOffset := updates[0].ID + 1

	for _, update := range updates[1:] {
		if update.ID >= nextOffset {
			nextOffset = update.ID + 1
		}
	}

	err = i.tx.Do(ctx, func(ctx context.Context) error {
		if createErr := i.inbox.CreateInboxItems(ctx, repository.CreateInboxItemsRequest{
			BotID:   lease.BotID,
			Updates: updates,
		}); createErr != nil {
			return createErr
		}

		return i.inbox.CommitIngestion(ctx, repository.CommitIngestionRequest{
			Lease:      *lease,
			NextOffset: nextOffset,
		})
	})
	if err != nil {
		return span.Err(err)
	}

	return nil
}
