package repository

import (
	"context"
	"encoding/json"
	"time"

	"tg-bot-proxy/internal/domain/gateway"
)

type InboxItem struct {
	ID           int64
	BotID        int64
	UpdateID     int64
	Payload      json.RawMessage
	AttemptCount int
}

type ListInboxRequest struct {
	BotID int64
	Limit int
	Now   time.Time
}

type FailInboxRequest struct {
	Lease   Lease
	Item    InboxItem
	Message string
}

type RetryInboxRequest struct {
	Lease       Lease
	Item        InboxItem
	AvailableAt time.Time
	Message     string
}

type CreateInboxItemsRequest struct {
	BotID   int64
	Updates []gateway.TelegramUpdate
}

type CommitIngestionRequest struct {
	Lease      Lease
	NextOffset int64
}

type Inbox interface {
	ClaimIngestion(context.Context, ClaimLeaseRequest) (*Lease, bool, error)
	CreateInboxItems(context.Context, CreateInboxItemsRequest) error
	CommitIngestion(context.Context, CommitIngestionRequest) error
	ReleaseIngestion(context.Context, Lease) error
	ClaimInboxProcessor(context.Context, ClaimLeaseRequest) (*Lease, bool, error)
	RenewInboxProcessor(context.Context, Lease, time.Time) error
	ListInbox(context.Context, ListInboxRequest) ([]InboxItem, error)
	DeleteInbox(context.Context, Lease, int64) error
	RetryInbox(context.Context, RetryInboxRequest) error
	FailInbox(context.Context, FailInboxRequest) error
	ReleaseInboxProcessor(context.Context, Lease) error
}
