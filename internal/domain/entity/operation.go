package entity

import "github.com/google/uuid"

type OperationType string

const (
	OperationRelayExternalToInternal OperationType = "relay_external_to_internal"
	OperationRelayInternalToExternal OperationType = "relay_internal_to_external"
	OperationSendExternalMessage     OperationType = "send_external_message"
)

type OperationStatus string

const (
	OperationPending    OperationStatus = "pending"
	OperationProcessing OperationStatus = "processing"
	OperationRetry      OperationStatus = "retry"
	OperationFailed     OperationStatus = "failed"
)

type OperationPayload interface {
	isOperationPayload()
}

type RelayExternalToInternalPayload struct {
	SourceChatID    int64
	SourceMessageID int64
	TopicTitle      string
}

func (RelayExternalToInternalPayload) isOperationPayload() {}

type RelayInternalToExternalPayload struct {
	SourceChatID    int64
	SourceMessageID int64
}

func (RelayInternalToExternalPayload) isOperationPayload() {}

type SendExternalMessagePayload struct {
	Text string
}

func (SendExternalMessagePayload) isOperationPayload() {}

// InvalidOperationPayload preserves a claimed operation whose persisted payload
// cannot be decoded so the application can record its terminal disposition.
type InvalidOperationPayload struct{}

func (InvalidOperationPayload) isOperationPayload() {}

type Operation struct {
	ID             int64
	ChatID         ChatID
	ChatOffset     int64
	Type           OperationType
	Payload        OperationPayload
	AttemptCount   int
	LeaseOwner     uuid.UUID
	LeaseVersion   int64
	ExternalChatID ExternalChatID
	SupergroupID   int64
	TopicID        *TopicID
}
