package repository

import (
	"context"
	"time"

	"tg-bot-proxy/internal/domain/entity"

	"github.com/google/uuid"
)

type NewOperation struct {
	DeduplicationKey string
	Type             entity.OperationType
	Payload          entity.OperationPayload
}

type Failure struct {
	Message           string
	HTTPStatus        *int
	ErrorCode         *int
	RetryAfterSeconds *int
}

type ClaimOperationsRequest struct {
	Limit      int
	Owner      uuid.UUID
	Now        time.Time
	LeaseUntil time.Time
}

type CreateOperationRequest struct {
	ChatID     entity.ChatID
	ChatOffset int64
	Operation  NewOperation
}

type RetryOperationRequest struct {
	Operation   entity.Operation
	AvailableAt time.Time
	Failure     Failure
}

type Outbox interface {
	CreateOperation(context.Context, CreateOperationRequest) error
	ClaimOperations(context.Context, ClaimOperationsRequest) ([]entity.Operation, error)
	LockOperation(context.Context, entity.Operation) error
	RenewOperation(context.Context, entity.Operation, time.Time) error
	CompleteOperation(context.Context, entity.Operation) error
	RetryOperation(context.Context, RetryOperationRequest) error
	FailOperation(context.Context, entity.Operation, Failure) error
}
