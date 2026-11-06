package gateway

import (
	"context"
	"fmt"
)

type TelegramUpdate struct {
	ID      int64
	Payload []byte
}

type DecodedUpdate struct {
	Message *DecodedMessage
}

type DecodedMessage struct {
	ID               int64
	ThreadID         int64
	ChatID           int64
	ChatType         string
	Text             string
	Username         string
	FirstName        string
	LastName         string
	FromBot          bool
	SupportedContent bool
}

type GetUpdatesRequest struct {
	Offset         int64
	Limit          int
	TimeoutSeconds int
	AllowedUpdates []string
}

type CopyMessageRequest struct {
	SourceChatID      int64
	SourceMessageID   int64
	DestinationChatID int64
	TopicID           *int64
}

type Telegram interface {
	GetUpdates(context.Context, GetUpdatesRequest) ([]TelegramUpdate, error)
	DecodeUpdate([]byte) (DecodedUpdate, error)
	CreateForumTopic(context.Context, string) (int64, error)
	CopyMessage(context.Context, CopyMessageRequest) error
	SendMessage(context.Context, int64, string) error
}

type TelegramOutcomeKind string

const (
	TelegramOutcomePermanent    TelegramOutcomeKind = "permanent"
	TelegramOutcomeRetryable    TelegramOutcomeKind = "retryable"
	TelegramOutcomeMissingTopic TelegramOutcomeKind = "missing_topic"
	TelegramOutcomeAmbiguous    TelegramOutcomeKind = "ambiguous"
)

type TelegramError struct {
	Code        int
	HTTPStatus  int
	Description string
	RetryAfter  int
	Kind        TelegramOutcomeKind
}

func (e *TelegramError) Error() string {
	return fmt.Sprintf("telegram error %d: %s", e.Code, e.Description)
}
