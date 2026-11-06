package telegram

import (
	"context"
	"errors"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"
)

type sendMessageRequest struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

func (i *Implementation) SendMessage(ctx context.Context, chatID int64, text string) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	if chatID == 0 || !validTelegramText(text, telegramMessageMaxRunes) {
		return span.Err(errors.Join(&proterror.InvalidArgument{}, errors.New("invalid Telegram request")))
	}

	var body telegramResponse[messageResult]
	response, err := i.client.R().
		SetContext(ctx).
		SetBody(sendMessageRequest{ChatID: chatID, Text: text}).
		SetResult(&body).
		SetError(&body).
		Post(methodPath(i.cfg.Bot.Token, "sendMessage"))
	if err = messageResponseError(response, body, err); err != nil {
		return span.Err(err)
	}

	return nil
}
