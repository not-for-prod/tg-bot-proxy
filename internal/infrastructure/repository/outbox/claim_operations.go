package outboxrepository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/domain/repository"
	"tg-bot-proxy/internal/infrastructure/repository/outbox/query"

	"github.com/not-for-prod/observer/tracer/prospan"
	proterrors "github.com/not-for-prod/proterror"
	"github.com/not-for-prod/proterror/proterror"
)

type claimRelayExternalToInternalPayload struct {
	SourceChatID    int64  `json:"sourceChatId"`
	SourceMessageID int64  `json:"sourceMessageId"`
	TopicTitle      string `json:"topicTitle"`
}

type claimRelayInternalToExternalPayload struct {
	SourceChatID    int64 `json:"sourceChatId"`
	SourceMessageID int64 `json:"sourceMessageId"`
}

type claimSendExternalMessagePayload struct {
	Text string `json:"text"`
}

func unmarshalOperationPayload(operationType entity.OperationType, encoded []byte) (entity.OperationPayload, error) {
	switch operationType {
	case entity.OperationRelayExternalToInternal:
		var value claimRelayExternalToInternalPayload

		if err := json.Unmarshal(encoded, &value); err != nil {
			return entity.InvalidOperationPayload{}, nil //nolint:nilerr // The application must persist terminal disposition.
		}

		return entity.RelayExternalToInternalPayload(value), nil
	case entity.OperationRelayInternalToExternal:
		var value claimRelayInternalToExternalPayload

		if err := json.Unmarshal(encoded, &value); err != nil {
			return entity.InvalidOperationPayload{}, nil //nolint:nilerr // The application must persist terminal disposition.
		}

		return entity.RelayInternalToExternalPayload(value), nil
	case entity.OperationSendExternalMessage:
		var value claimSendExternalMessagePayload

		if err := json.Unmarshal(encoded, &value); err != nil {
			return entity.InvalidOperationPayload{}, nil //nolint:nilerr // The application must persist terminal disposition.
		}

		return entity.SendExternalMessagePayload(value), nil
	default:
		return nil, errors.Join(&proterror.Internal{}, fmt.Errorf("invalid %q operation payload", operationType))
	}
}

func (i *Implementation) ClaimOperations(
	ctx context.Context,
	req repository.ClaimOperationsRequest,
) ([]entity.Operation, error) {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	rows, err := i.tr(ctx).Query(ctx, query.ClaimOperations, req.Now, req.Limit, req.Owner, req.LeaseUntil)
	if err != nil {
		return nil, span.Err(proterrors.FromPG(err))
	}
	defer rows.Close()

	operations := make([]entity.Operation, 0, req.Limit)

	for rows.Next() {
		var op entity.Operation
		var payload json.RawMessage
		var topic *int64

		if err = rows.Scan(&op.ID, &op.ChatID, &op.ChatOffset, &op.Type, &payload, &op.AttemptCount, &op.LeaseOwner, &op.LeaseVersion, &op.ExternalChatID, &op.SupergroupID, &topic); err != nil {
			return nil, span.Err(proterrors.FromPG(err))
		}

		op.Payload, err = unmarshalOperationPayload(op.Type, payload)
		if err != nil {
			return nil, span.Err(err)
		}

		if topic != nil {
			value := entity.TopicID(*topic)
			op.TopicID = &value
		}

		operations = append(operations, op)
	}

	if err = rows.Err(); err != nil {
		return nil, span.Err(proterrors.FromPG(err))
	}

	return operations, nil
}
