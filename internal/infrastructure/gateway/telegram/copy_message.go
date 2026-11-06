package telegram

import (
	"context"
	"errors"

	"tg-bot-proxy/internal/domain/gateway"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"
)

type copyMessageRequest struct {
	ChatID          int64                       `json:"chat_id"`
	FromChatID      int64                       `json:"from_chat_id"`
	MessageID       int64                       `json:"message_id"`
	MessageThreadID *int64                      `json:"message_thread_id,omitempty"`
	ReplyParameters *copyMessageReplyParameters `json:"reply_parameters,omitempty"`
}

type copyMessageReplyParameters struct {
	MessageID                int64 `json:"message_id"`
	AllowSendingWithoutReply bool  `json:"allow_sending_without_reply"`
}

func (i *Implementation) CopyMessage(ctx context.Context, req gateway.CopyMessageRequest) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()
	if req.SourceChatID == 0 || req.DestinationChatID == 0 || req.SourceMessageID <= 0 ||
		req.TopicID != nil && *req.TopicID <= 0 {
		return span.Err(errors.Join(&proterror.InvalidArgument{}, errors.New("invalid Telegram request")))
	}

	request := copyMessageRequest{
		ChatID:          req.DestinationChatID,
		FromChatID:      req.SourceChatID,
		MessageID:       req.SourceMessageID,
		MessageThreadID: req.TopicID,
	}
	if req.TopicID != nil {
		request.ReplyParameters = &copyMessageReplyParameters{
			MessageID:                *req.TopicID,
			AllowSendingWithoutReply: false,
		}
	}

	var body telegramResponse[messageResult]
	response, err := i.client.R().
		SetContext(ctx).
		SetBody(request).
		SetResult(&body).
		SetError(&body).
		Post(methodPath(i.cfg.Bot.Token, "copyMessage"))
	if err = messageResponseError(response, body, err); err != nil {
		return span.Err(err)
	}

	return nil
}
