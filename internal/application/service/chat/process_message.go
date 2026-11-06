package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/domain/gateway"
	"tg-bot-proxy/internal/domain/repository"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"
)

func (i *Implementation) processInboxItem(
	ctx context.Context,
	item repository.InboxItem,
	lease repository.Lease,
) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	var terminalErr error

	err := i.tx.Do(ctx, func(ctx context.Context) error {
		update, decodeErr := i.telegram.DecodeUpdate(item.Payload)

		if decodeErr != nil {
			if failErr := i.inbox.FailInbox(ctx, repository.FailInboxRequest{
				Lease:   lease,
				Item:    item,
				Message: decodeErr.Error(),
			}); failErr != nil {
				return failErr
			}

			terminalErr = decodeErr

			return nil
		}

		if update.Message == nil {
			return i.inbox.DeleteInbox(ctx, lease, item.ID)
		}

		return i.processMessage(ctx, lease, item, update.Message)
	})
	if err == nil {
		if terminalErr != nil {
			i.recordInboxFailureObservation(&span, item, terminalErr)
		}

		return nil
	}

	if item.AttemptCount+1 >= i.cfg.Inbox.Retry.MaxAttempts {
		if failErr := i.inbox.FailInbox(ctx, repository.FailInboxRequest{
			Lease:   lease,
			Item:    item,
			Message: err.Error(),
		}); failErr != nil {
			return span.Err(failErr)
		}

		i.recordInboxFailureObservation(&span, item, err)

		return nil
	}

	if retryErr := i.inbox.RetryInbox(
		ctx,
		repository.RetryInboxRequest{
			Lease:       lease,
			Item:        item,
			AvailableAt: retryAt(i.cfg.Inbox.Retry, item.AttemptCount+1, time.Now().UTC()),
			Message:     err.Error(),
		},
	); retryErr != nil {
		return span.Err(retryErr)
	}

	return span.Err(err)
}

func (i *Implementation) recordInboxFailureObservation(
	span *prospan.ProSpan,
	item repository.InboxItem,
	processingErr error,
) {
	span.Logger().Info(
		"inbox item failure disposition recorded",
		"disposition", inboxDispositionFailed,
		"bot_id", item.BotID,
		"inbox_item_id", item.ID,
		"update_id", item.UpdateID,
		"attempt", item.AttemptCount,
	)

	if i.metrics != nil {
		i.metrics.inboxDispositions.WithLabelValues(inboxDispositionFailed).Inc()
	}

	_ = span.Err(processingErr)
}

func (i *Implementation) processMessage(
	ctx context.Context,
	lease repository.Lease,
	item repository.InboxItem,
	message *gateway.DecodedMessage,
) error {
	if !message.SupportedContent || message.FromBot {
		return i.inbox.DeleteInbox(ctx, lease, item.ID)
	}

	if message.ChatType == "private" {
		return i.processExternalMessage(ctx, lease, item, message)
	}

	if message.ChatID != i.cfg.Telegram.SupergroupID || message.ThreadID <= 0 || strings.HasPrefix(message.Text, "/") {
		return i.inbox.DeleteInbox(ctx, lease, item.ID)
	}

	chat, err := i.chats.GetChatByTopic(ctx, message.ChatID, entity.TopicID(message.ThreadID))
	if err != nil {
		if errors.Is(err, &proterror.NotFound{}) {
			return i.inbox.DeleteInbox(ctx, lease, item.ID)
		}

		return err
	}

	operation := repository.NewOperation{
		DeduplicationKey: fmt.Sprintf("internal:%d", item.UpdateID),
		Type:             entity.OperationRelayInternalToExternal,
		Payload: entity.RelayInternalToExternalPayload{
			SourceChatID:    message.ChatID,
			SourceMessageID: message.ID,
		},
	}

	offset := chat.AllocateOffset()

	if err = i.chats.UpdateChat(ctx, chat); err != nil {
		return err
	}

	if err = i.outbox.CreateOperation(ctx, repository.CreateOperationRequest{
		ChatID:     chat.ID,
		ChatOffset: offset,
		Operation:  operation,
	}); err != nil {
		return err
	}

	return i.inbox.DeleteInbox(ctx, lease, item.ID)
}

func (i *Implementation) processExternalMessage(
	ctx context.Context,
	lease repository.Lease,
	item repository.InboxItem,
	message *gateway.DecodedMessage,
) error {
	if message.ChatType != "private" {
		return i.inbox.DeleteInbox(ctx, lease, item.ID)
	}

	isCommand := strings.HasPrefix(message.Text, "/")

	if isCommand && !isStartCommand(message.Text) {
		return i.inbox.DeleteInbox(ctx, lease, item.ID)
	}

	chat, err := i.chats.GetChatByExternalID(ctx, entity.ExternalChatID(message.ChatID))
	if err != nil {
		if !errors.Is(err, &proterror.NotFound{}) {
			return err
		}

		chat = entity.NewChat(entity.ExternalChatID(message.ChatID), i.cfg.Telegram.SupergroupID)

		if err = i.chats.CreateChat(ctx, chat); err != nil {
			return err
		}
	}

	var operation repository.NewOperation

	if isCommand {
		operation = repository.NewOperation{
			DeduplicationKey: fmt.Sprintf("external:%d", item.UpdateID),
			Type:             entity.OperationSendExternalMessage,
			Payload: entity.SendExternalMessagePayload{
				Text: i.cfg.Telegram.StartMessage,
			},
		}
	} else {
		title := topicTitle(message)

		operation = repository.NewOperation{
			DeduplicationKey: fmt.Sprintf("external:%d", item.UpdateID),
			Type:             entity.OperationRelayExternalToInternal,
			Payload: entity.RelayExternalToInternalPayload{
				SourceChatID:    message.ChatID,
				SourceMessageID: message.ID,
				TopicTitle:      title,
			},
		}
	}

	offset := chat.AllocateOffset()

	if err = i.chats.UpdateChat(ctx, chat); err != nil {
		return err
	}

	if err = i.outbox.CreateOperation(ctx, repository.CreateOperationRequest{
		ChatID:     chat.ID,
		ChatOffset: offset,
		Operation:  operation,
	}); err != nil {
		return err
	}

	return i.inbox.DeleteInbox(ctx, lease, item.ID)
}

func topicTitle(message *gateway.DecodedMessage) string {
	var title string

	if message.Username != "" {
		title = "@" + message.Username
	} else if name := strings.TrimSpace(message.FirstName + " " + message.LastName); name != "" {
		title = name
	} else {
		title = fmt.Sprintf("User %d", message.ChatID)
	}

	const maxTopicTitleLength = 128
	titleRunes := []rune(title)

	if len(titleRunes) > maxTopicTitleLength {
		title = string(titleRunes[:maxTopicTitleLength])
	}

	return title
}

func isStartCommand(text string) bool {
	fields := strings.Fields(text)

	if len(fields) == 0 {
		return false
	}

	command, _, _ := strings.Cut(fields[0], "@")

	return command == "/start"
}
