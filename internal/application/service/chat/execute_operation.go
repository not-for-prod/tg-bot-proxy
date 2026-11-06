package chat

import (
	"context"
	"errors"
	"time"

	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/domain/gateway"
	"tg-bot-proxy/internal/domain/repository"

	"github.com/not-for-prod/observer/tracer/prospan"
	"github.com/not-for-prod/proterror/proterror"
)

const operationFinalizationTimeout = 5 * time.Second

func (i *Implementation) ExecuteOperation(ctx context.Context, op entity.Operation) error {
	ctx, span := prospan.Start(ctx)
	defer span.End()

	executionCtx, cancelExecution := context.WithCancel(ctx)
	defer cancelExecution()
	stopRenewal := make(chan struct{})
	renewalResult := make(chan error, 1)
	leaseOperation := op

	go func() {
		renewalResult <- i.renewOperationLease(executionCtx, cancelExecution, stopRenewal, leaseOperation)
	}()

	executionErr := i.executeOperation(executionCtx, &op)
	close(stopRenewal)
	leaseErr := <-renewalResult

	if leaseErr != nil {
		if errors.Is(leaseErr, repository.ErrStaleLease) {
			return span.Err(leaseErr)
		}

		var telegramErr *gateway.TelegramError

		if !errors.As(executionErr, &telegramErr) || telegramErr.Kind != gateway.TelegramOutcomeAmbiguous {
			return span.Err(leaseErr)
		}
	}

	var telegramErr *gateway.TelegramError

	if errors.As(executionErr, &telegramErr) && telegramErr.Kind == gateway.TelegramOutcomeAmbiguous {
		finalizationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), operationFinalizationTimeout)
		defer cancel()
		ctx = finalizationCtx
	}

	if executionErr == nil {
		err := i.tx.Do(ctx, func(ctx context.Context) error {
			if completeErr := i.outbox.CompleteOperation(ctx, op); completeErr != nil {
				return completeErr
			}

			return i.chats.CommitChatOffset(ctx, op.ChatID, op.ChatOffset)
		})
		if err != nil {
			return span.Err(err)
		}

		if i.metrics != nil {
			i.metrics.recordOutboxDisposition(op.Type, operationDispositionCompleted)
		}

		return nil
	}

	return i.recordOperationFailure(ctx, &span, op, executionErr)
}

func (i *Implementation) recordOperationFailure(
	ctx context.Context,
	span *prospan.ProSpan,
	op entity.Operation,
	executionErr error,
) error {
	failure := failureFromError(executionErr)
	httpStatus := 0

	if failure.HTTPStatus != nil {
		httpStatus = *failure.HTTPStatus
	}

	errorCode := 0

	if failure.ErrorCode != nil {
		errorCode = *failure.ErrorCode
	}

	var telegramErr *gateway.TelegramError
	kind := gateway.TelegramOutcomePermanent

	if errors.As(executionErr, &telegramErr) {
		kind = telegramErr.Kind
	}

	switch kind {
	case gateway.TelegramOutcomeRetryable:
		if op.AttemptCount < i.cfg.Outbox.Retry.MaxAttempts {
			available := retryAt(i.cfg.Outbox.Retry, op.AttemptCount, time.Now().UTC())

			if telegramErr.RetryAfter > 0 {
				available = time.Now().UTC().Add(time.Duration(telegramErr.RetryAfter) * time.Second)
			}

			if err := i.outbox.RetryOperation(ctx, repository.RetryOperationRequest{
				Operation:   op,
				AvailableAt: available,
				Failure:     failure,
			}); err != nil {
				return span.Err(err)
			}

			span.Logger().Info(
				"outbox operation disposition recorded",
				"disposition", "retry",
				"operation_id", op.ID,
				"chat_id", op.ChatID,
				"operation_type", op.Type,
				"attempt", op.AttemptCount,
				"http_status", httpStatus,
				"error_code", errorCode,
				"retry_at", available,
			)

			i.recordOperationFailureObservation(span, op, operationDispositionRetry, executionErr)

			return nil
		}
	case gateway.TelegramOutcomePermanent,
		gateway.TelegramOutcomeMissingTopic,
		gateway.TelegramOutcomeAmbiguous:
	default:
	}

	if err := i.outbox.FailOperation(ctx, op, failure); err != nil {
		return span.Err(err)
	}

	span.Logger().Info(
		"outbox operation disposition recorded",
		"disposition", "failed",
		"operation_id", op.ID,
		"chat_id", op.ChatID,
		"operation_type", op.Type,
		"attempt", op.AttemptCount,
		"http_status", httpStatus,
		"error_code", errorCode,
	)

	i.recordOperationFailureObservation(span, op, operationDispositionFailed, executionErr)

	return nil
}

func (i *Implementation) recordOperationFailureObservation(
	span *prospan.ProSpan,
	op entity.Operation,
	disposition string,
	executionErr error,
) {
	if i.metrics != nil {
		i.metrics.recordOutboxDisposition(op.Type, disposition)
	}

	_ = span.Err(executionErr)
}

func (i *Implementation) executeOperation(ctx context.Context, operation *entity.Operation) error {
	switch operation.Type {
	case entity.OperationRelayExternalToInternal:
		return i.relayExternalToInternal(ctx, operation)

	case entity.OperationRelayInternalToExternal:
		payload, ok := operation.Payload.(entity.RelayInternalToExternalPayload)
		if !ok {
			return errors.Join(&proterror.Internal{}, errors.New("invalid internal relay operation payload"))
		}

		return i.telegram.CopyMessage(
			ctx,
			gateway.CopyMessageRequest{
				SourceChatID:      payload.SourceChatID,
				SourceMessageID:   payload.SourceMessageID,
				DestinationChatID: int64(operation.ExternalChatID),
			},
		)

	case entity.OperationSendExternalMessage:
		payload, ok := operation.Payload.(entity.SendExternalMessagePayload)
		if !ok {
			return errors.Join(&proterror.Internal{}, errors.New("invalid send-message operation payload"))
		}

		return i.telegram.SendMessage(ctx, int64(operation.ExternalChatID), payload.Text)

	default:
		return errors.New("unsupported outbox operation")
	}
}

func (i *Implementation) relayExternalToInternal(ctx context.Context, operation *entity.Operation) error {
	payload, ok := operation.Payload.(entity.RelayExternalToInternalPayload)
	if !ok {
		return errors.Join(&proterror.Internal{}, errors.New("invalid external relay operation payload"))
	}

	if operation.TopicID == nil {
		topicID, err := i.telegram.CreateForumTopic(ctx, payload.TopicTitle)

		if err != nil {
			return err
		}

		if err = i.storeTopic(ctx, operation, topicID); err != nil {
			return err
		}
	}

	initialTopicID := int64(*operation.TopicID)
	request := gateway.CopyMessageRequest{
		SourceChatID:      payload.SourceChatID,
		SourceMessageID:   payload.SourceMessageID,
		DestinationChatID: operation.SupergroupID,
		TopicID:           &initialTopicID,
	}

	err := i.telegram.CopyMessage(ctx, request)
	var telegramErr *gateway.TelegramError

	if !errors.As(err, &telegramErr) || telegramErr.Kind != gateway.TelegramOutcomeMissingTopic {
		return err
	}

	replacementTopicID, err := i.telegram.CreateForumTopic(ctx, payload.TopicTitle)

	if err != nil {
		return err
	}

	if err = i.storeTopic(ctx, operation, replacementTopicID); err != nil {
		return err
	}

	request.TopicID = &replacementTopicID

	return i.telegram.CopyMessage(ctx, request)
}

func (i *Implementation) renewOperationLease(
	ctx context.Context,
	cancel context.CancelFunc,
	stop <-chan struct{},
	operation entity.Operation,
) error {
	const renewalIntervalsPerLease = 3

	interval := i.cfg.Outbox.LeaseDuration / renewalIntervalsPerLease
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stop:
			return nil
		case <-ticker.C:
			until := time.Now().UTC().Add(i.cfg.Outbox.LeaseDuration)

			if err := i.outbox.RenewOperation(ctx, operation, until); err != nil {
				cancel()
				return err
			}
		}
	}
}

func (i *Implementation) storeTopic(ctx context.Context, op *entity.Operation, topicID int64) error {
	return i.tx.Do(ctx, func(ctx context.Context) error {
		if err := i.outbox.LockOperation(ctx, *op); err != nil {
			return err
		}

		chat, err := i.chats.GetChatByExternalID(ctx, op.ExternalChatID)
		if err != nil {
			return err
		}

		if err = chat.MapTopic(entity.TopicID(topicID)); err != nil {
			return err
		}

		if err = i.chats.UpdateChat(ctx, chat); err != nil {
			return err
		}

		value := entity.TopicID(topicID)
		op.TopicID = &value

		return nil
	})
}

func failureFromError(err error) repository.Failure {
	failure := repository.Failure{Message: err.Error()}
	var telegramErr *gateway.TelegramError

	if errors.As(err, &telegramErr) {
		if telegramErr.HTTPStatus > 0 {
			status := telegramErr.HTTPStatus
			failure.HTTPStatus = &status
		}

		code := telegramErr.Code
		failure.ErrorCode = &code

		if telegramErr.RetryAfter > 0 {
			retry := telegramErr.RetryAfter
			failure.RetryAfterSeconds = &retry
		}
	}

	return failure
}
