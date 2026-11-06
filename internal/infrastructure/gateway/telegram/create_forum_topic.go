package telegram

import (
	"context"
	"errors"
	"net"
	"net/http"

	"tg-bot-proxy/internal/domain/gateway"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"
)

type createForumTopicRequest struct {
	ChatID int64  `json:"chat_id"`
	Name   string `json:"name"`
}

type createForumTopicResult struct {
	MessageThreadID int64 `json:"message_thread_id"`
}

func (i *Implementation) CreateForumTopic(ctx context.Context, title string) (int64, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	if i.cfg.SupergroupID == 0 || !validTelegramText(title, telegramTopicMaxRunes) {
		return 0, span.Err(errors.Join(&proterror.InvalidArgument{}, errors.New("invalid Telegram request")))
	}

	var body telegramResponse[createForumTopicResult]
	response, err := i.client.R().
		SetContext(ctx).
		SetBody(createForumTopicRequest{ChatID: i.cfg.SupergroupID, Name: title}).
		SetResult(&body).
		SetError(&body).
		Post(methodPath(i.cfg.Bot.Token, "createForumTopic"))
	if err != nil {
		var dnsErr *net.DNSError
		var operationErr *net.OpError
		if errors.As(err, &dnsErr) || errors.As(err, &operationErr) && operationErr.Op == "dial" {
			return 0, span.Err(retryableTransportError())
		}

		return 0, span.Err(errors.Join(
			&proterror.Unavailable{},
			&gateway.TelegramError{
				Description: "topic creation outcome is unknown",
				Kind:        gateway.TelegramOutcomeAmbiguous,
			},
		))
	}
	if !body.OK {
		if response.StatusCode() >= http.StatusInternalServerError {
			code := body.ErrorCode
			if code == 0 {
				code = response.StatusCode()
			}
			return 0, span.Err(errors.Join(
				&proterror.Unavailable{},
				&gateway.TelegramError{
					Code:        code,
					HTTPStatus:  response.StatusCode(),
					Description: safeTelegramErrorDescription(gateway.TelegramOutcomeAmbiguous),
					Kind:        gateway.TelegramOutcomeAmbiguous,
				},
			))
		}

		return 0, span.Err(
			telegramAPIError(response.StatusCode(), body.ErrorCode, body.Description, body.Parameters.RetryAfter),
		)
	}
	if body.Result.MessageThreadID <= 0 {
		return 0, span.Err(errors.Join(&proterror.Internal{}, errors.New("telegram returned an invalid topic ID")))
	}

	return body.Result.MessageThreadID, nil
}
