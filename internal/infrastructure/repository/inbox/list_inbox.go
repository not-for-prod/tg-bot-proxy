package inboxrepository

import (
	"context"
	"errors"
	"tg-bot-proxy/internal/domain/repository"
	"tg-bot-proxy/internal/generated/xo"

	sq "github.com/Masterminds/squirrel"
	"github.com/not-for-prod/observer/tracer/prospan"
	proterrors "github.com/not-for-prod/proterror"
	"github.com/not-for-prod/proterror/proterror"
)

func (i *Implementation) ListInbox(
	ctx context.Context,
	req repository.ListInboxRequest,
) ([]repository.InboxItem, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	query, args, err := sq.Select(
		xo.Field_Inbox_ID,
		xo.Field_Inbox_BotID,
		xo.Field_Inbox_UpdateID,
		xo.Field_Inbox_Payload,
		xo.Field_Inbox_AttemptCount,
	).
		From(xo.Table_Inbox).
		Where(sq.Eq{
			xo.Field_Inbox_BotID:  req.BotID,
			xo.Field_Inbox_Status: []string{"pending", "retry"},
		}).
		Where(sq.LtOrEq{xo.Field_Inbox_AvailableAt: req.Now}).
		OrderBy(xo.Field_Inbox_UpdateID).
		Limit(uint64(req.Limit)). //nolint:gosec // Config validation bounds the positive batch size.
		Suffix("FOR UPDATE").
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return nil, span.Err(errors.Join(&proterror.Internal{}, err))
	}

	rows, err := i.tr(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, span.Err(proterrors.FromPG(err))
	}
	defer rows.Close()

	items := make([]repository.InboxItem, 0, req.Limit)

	for rows.Next() {
		var item repository.InboxItem

		if err = rows.Scan(&item.ID, &item.BotID, &item.UpdateID, &item.Payload, &item.AttemptCount); err != nil {
			return nil, span.Err(proterrors.FromPG(err))
		}

		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		return nil, span.Err(proterrors.FromPG(err))
	}

	return items, nil
}
