package outboxrepository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/domain/repository"
	"tg-bot-proxy/internal/generated/xo"

	sq "github.com/Masterminds/squirrel"
	"github.com/not-for-prod/observer/tracer/prospan"
	proterrors "github.com/not-for-prod/proterror"
	"github.com/not-for-prod/proterror/proterror"
)

type createRelayExternalToInternalPayload struct {
	SourceChatID    int64  `json:"sourceChatId"`
	SourceMessageID int64  `json:"sourceMessageId"`
	TopicTitle      string `json:"topicTitle"`
}

type createRelayInternalToExternalPayload struct {
	SourceChatID    int64 `json:"sourceChatId"`
	SourceMessageID int64 `json:"sourceMessageId"`
}

type createSendExternalMessagePayload struct {
	Text string `json:"text"`
}

func marshalOperationPayload(operationType entity.OperationType, payload entity.OperationPayload) ([]byte, error) {
	var value any

	switch operationType {
	case entity.OperationRelayExternalToInternal:
		typed, ok := payload.(entity.RelayExternalToInternalPayload)
		if !ok {
			return nil, errors.Join(&proterror.Internal{}, fmt.Errorf("invalid %q operation payload", operationType))
		}

		value = createRelayExternalToInternalPayload(typed)
	case entity.OperationRelayInternalToExternal:
		typed, ok := payload.(entity.RelayInternalToExternalPayload)
		if !ok {
			return nil, errors.Join(&proterror.Internal{}, fmt.Errorf("invalid %q operation payload", operationType))
		}

		value = createRelayInternalToExternalPayload(typed)
	case entity.OperationSendExternalMessage:
		typed, ok := payload.(entity.SendExternalMessagePayload)
		if !ok {
			return nil, errors.Join(&proterror.Internal{}, fmt.Errorf("invalid %q operation payload", operationType))
		}

		value = createSendExternalMessagePayload(typed)
	default:
		return nil, errors.Join(&proterror.Internal{}, fmt.Errorf("invalid %q operation payload", operationType))
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, errors.Join(&proterror.Internal{}, err)
	}

	return encoded, nil
}

func (i *Implementation) CreateOperation(
	ctx context.Context,
	req repository.CreateOperationRequest,
) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	payload, err := marshalOperationPayload(req.Operation.Type, req.Operation.Payload)
	if err != nil {
		return span.Err(err)
	}

	query, args, err := sq.Insert(xo.Table_Outbox).
		Columns(
			xo.Field_Outbox_ChatID,
			xo.Field_Outbox_ChatOffset,
			xo.Field_Outbox_DeduplicationKey,
			xo.Field_Outbox_Operation,
			xo.Field_Outbox_Payload,
		).
		Values(req.ChatID, req.ChatOffset, req.Operation.DeduplicationKey, req.Operation.Type, payload).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return span.Err(errors.Join(&proterror.Internal{}, err))
	}

	if _, err = i.tr(ctx).Exec(ctx, query, args...); err != nil {
		return span.Err(proterrors.FromPG(err))
	}

	return nil
}
