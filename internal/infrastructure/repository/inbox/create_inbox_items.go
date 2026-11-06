package inboxrepository

import (
	"context"

	"tg-bot-proxy/internal/domain/repository"
	"tg-bot-proxy/internal/infrastructure/repository/inbox/query"

	"github.com/not-for-prod/observer/tracer/prospan"
	proterrors "github.com/not-for-prod/proterror"
)

func (i *Implementation) CreateInboxItems(ctx context.Context, req repository.CreateInboxItemsRequest) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	if len(req.Updates) == 0 {
		return nil
	}

	updateIDs := make([]int64, len(req.Updates))
	payloads := make([]string, len(req.Updates))

	for index, update := range req.Updates {
		updateIDs[index] = update.ID
		payloads[index] = string(update.Payload)
	}

	if _, err := i.tr(ctx).Exec(ctx, query.CreateInboxItems, req.BotID, updateIDs, payloads); err != nil {
		return span.Err(proterrors.FromPG(err))
	}

	return nil
}
