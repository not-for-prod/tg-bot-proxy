package chat

import (
	"context"
	"errors"
	"time"

	"tg-bot-proxy/internal/domain/repository"

	"github.com/google/uuid"
	"github.com/not-for-prod/observer/tracer/prospan"
)

func (i *Implementation) ProcessInboxBatch(ctx context.Context) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	botConfig := i.cfg.Telegram.Bot

	lease, claimed, err := i.inbox.ClaimInboxProcessor(
		ctx,
		repository.ClaimLeaseRequest{
			BotID:      botConfig.ID,
			Owner:      uuid.New(),
			LeaseUntil: time.Now().UTC().Add(i.cfg.Inbox.LeaseDuration),
		},
	)
	if err != nil {
		return span.Err(err)
	}

	if !claimed {
		return nil
	}

	var items []repository.InboxItem

	err = i.tx.Do(ctx, func(ctx context.Context) error {
		var listErr error

		items, listErr = i.inbox.ListInbox(ctx, repository.ListInboxRequest{
			BotID: botConfig.ID,
			Limit: i.cfg.Inbox.BatchSize,
			Now:   time.Now().UTC(),
		})

		return listErr
	})
	if err != nil {
		return span.Err(errors.Join(err, i.inbox.ReleaseInboxProcessor(ctx, *lease)))
	}

	batchErrors := make([]error, 0)

	for _, item := range items {
		until := time.Now().UTC().Add(i.cfg.Inbox.LeaseDuration)

		if err = i.inbox.RenewInboxProcessor(ctx, *lease, until); err != nil {
			batchErrors = append(batchErrors, err)
			break
		}

		if err = i.processInboxItem(ctx, item, *lease); err != nil {
			batchErrors = append(batchErrors, err)
		}
	}

	if err = i.inbox.ReleaseInboxProcessor(ctx, *lease); err != nil {
		batchErrors = append(batchErrors, err)
	}

	err = errors.Join(batchErrors...)
	if err != nil {
		return span.Err(err)
	}

	return nil
}
