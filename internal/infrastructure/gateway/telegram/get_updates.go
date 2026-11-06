package telegram

import (
	"context"
	"encoding/json"
	"errors"

	"tg-bot-proxy/internal/domain/gateway"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"
)

type getUpdatesRequest struct {
	Offset         int64    `json:"offset"`
	Limit          int      `json:"limit"`
	Timeout        int      `json:"timeout"`
	AllowedUpdates []string `json:"allowed_updates"`
}

func (i *Implementation) GetUpdates(
	ctx context.Context,
	req gateway.GetUpdatesRequest,
) ([]gateway.TelegramUpdate, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	var body telegramResponse[[]json.RawMessage]
	response, err := i.client.R().
		SetContext(ctx).
		SetBody(getUpdatesRequest{Offset: req.Offset, Limit: req.Limit, Timeout: req.TimeoutSeconds, AllowedUpdates: req.AllowedUpdates}).
		SetResult(&body).
		SetError(&body).
		Post(methodPath(i.cfg.Bot.Token, "getUpdates"))
	if err != nil {
		return nil, span.Err(retryableTransportError())
	}
	if !body.OK {
		return nil, span.Err(
			telegramAPIError(response.StatusCode(), body.ErrorCode, body.Description, body.Parameters.RetryAfter),
		)
	}
	if body.Result == nil {
		return nil, span.Err(malformedTelegramResponse())
	}

	updates := make([]gateway.TelegramUpdate, 0, len(body.Result))
	for _, raw := range body.Result {
		var update struct {
			ID *int64 `json:"update_id"`
		}
		if err = json.Unmarshal(raw, &update); err != nil {
			return nil, span.Err(errors.Join(&proterror.Internal{}, err))
		}
		if update.ID == nil || *update.ID < 0 {
			return nil, span.Err(
				errors.Join(&proterror.Internal{}, errors.New("telegram update has an invalid update_id")),
			)
		}
		updates = append(updates, gateway.TelegramUpdate{ID: *update.ID, Payload: raw})
	}

	return updates, nil
}
