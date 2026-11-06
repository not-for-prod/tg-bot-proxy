package chat //nolint:testpackage // topicTitle is intentionally package-private policy.

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"tg-bot-proxy/config"
	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/domain/gateway"
	"tg-bot-proxy/internal/domain/repository"

	"github.com/google/uuid"
	"github.com/not-for-prod/proterror/proterror"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

//nolint:gocognit // The table keeps the complete routing policy and its assertions visible.
func TestProcessInboxRoutesMessages(t *testing.T) {
	t.Parallel()

	const (
		externalChatID entity.ExternalChatID = 101
		supergroupID                         = int64(-100)
		topicID        entity.TopicID        = 55
	)

	externalChat, err := entity.RehydrateChat(7, externalChatID, supergroupID, nil, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	mappedTopicID := topicID
	internalChat, err := entity.RehydrateChat(8, 102, supergroupID, &mappedTopicID, 9, 6)
	if err != nil {
		t.Fatal(err)
	}

	externalRelay := repository.NewOperation{
		DeduplicationKey: "external:17",
		Type:             entity.OperationRelayExternalToInternal,
		Payload: entity.RelayExternalToInternalPayload{
			SourceChatID: 101, SourceMessageID: 202, TopicTitle: "@support",
		},
	}
	startReply := repository.NewOperation{
		DeduplicationKey: "external:17",
		Type:             entity.OperationSendExternalMessage,
		Payload:          entity.SendExternalMessagePayload{Text: "Welcome"},
	}
	internalRelay := repository.NewOperation{
		DeduplicationKey: "internal:17",
		Type:             entity.OperationRelayInternalToExternal,
		Payload:          entity.RelayInternalToExternalPayload{SourceChatID: -100, SourceMessageID: 303},
	}

	tests := []struct {
		name                string
		message             *gateway.DecodedMessage
		externalChat        *entity.Chat
		externalLookupErr   error
		topicChat           *entity.Chat
		topicLookupErr      error
		wantExternalLookup  bool
		wantTopicLookup     bool
		wantCreate          bool
		wantUpdatedOffset   int64
		wantOperation       *repository.NewOperation
		wantOperationChatID entity.ChatID
		wantOperationOffset int64
	}{
		{
			name: "valid external relay",
			message: &gateway.DecodedMessage{
				ID: 202, ChatID: int64(externalChatID), ChatType: "private", Username: "support", SupportedContent: true,
			},
			externalChat: externalChat, wantExternalLookup: true, wantUpdatedOffset: 5,
			wantOperation: &externalRelay, wantOperationChatID: 7, wantOperationOffset: 4,
		},
		{
			name: "start creates chat and reply",
			message: &gateway.DecodedMessage{
				ID: 203, ChatID: int64(externalChatID), ChatType: "private", Text: "/start@external_bot extra",
				SupportedContent: true,
			},
			externalLookupErr: &proterror.NotFound{}, wantExternalLookup: true, wantCreate: true,
			wantUpdatedOffset: 2, wantOperation: &startReply, wantOperationChatID: 41, wantOperationOffset: 1,
		},
		{
			name: "ignored external command",
			message: &gateway.DecodedMessage{
				ChatID: int64(externalChatID), ChatType: "private", Text: "/help", SupportedContent: true,
			},
		},
		{
			name: "external non-private message",
			message: &gateway.DecodedMessage{
				ChatID: int64(externalChatID), ChatType: "group", SupportedContent: true,
			},
		},
		{
			name: "valid mapped internal relay",
			message: &gateway.DecodedMessage{
				ID: 303, ChatID: supergroupID, ChatType: "supergroup", ThreadID: int64(topicID), SupportedContent: true,
			},
			topicChat: internalChat, wantTopicLookup: true, wantUpdatedOffset: 10,
			wantOperation: &internalRelay, wantOperationChatID: 8, wantOperationOffset: 9,
		},
		{
			name: "internal command",
			message: &gateway.DecodedMessage{
				ChatID: supergroupID, ChatType: "supergroup", ThreadID: int64(topicID), Text: "/close", SupportedContent: true,
			},
		},
		{
			name: "wrong internal supergroup",
			message: &gateway.DecodedMessage{
				ChatID: -200, ChatType: "supergroup", ThreadID: int64(topicID), SupportedContent: true,
			},
		},
		{
			name: "internal message without topic",
			message: &gateway.DecodedMessage{
				ChatID: supergroupID, ChatType: "supergroup", SupportedContent: true,
			},
		},
		{
			name: "unmapped internal topic",
			message: &gateway.DecodedMessage{
				ChatID: supergroupID, ChatType: "supergroup", ThreadID: int64(topicID), SupportedContent: true,
			},
			topicLookupErr: &proterror.NotFound{}, wantTopicLookup: true,
		},
		{name: "no message"},
		{
			name:    "service message",
			message: &gateway.DecodedMessage{ChatID: supergroupID, ChatType: "supergroup", ThreadID: int64(topicID)},
		},
		{
			name: "bot message",
			message: &gateway.DecodedMessage{
				ChatID: supergroupID, ChatType: "supergroup", ThreadID: int64(topicID), FromBot: true,
				SupportedContent: true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			lease := repository.Lease{BotID: 5, Owner: uuid.New(), Version: 7, Offset: 11}
			item := repository.InboxItem{ID: 13, UpdateID: 17}
			repo := &processInboxRepository{
				externalChat:      test.externalChat,
				externalLookupErr: test.externalLookupErr,
				topicChat:         test.topicChat,
				topicLookupErr:    test.topicLookupErr,
			}
			implementation := &Implementation{
				cfg: &config.Config{Telegram: config.Telegram{
					SupergroupID: supergroupID,
					StartMessage: "Welcome",
				}},
				inbox: repo, chats: repo, outbox: repo,
				telegram: decodedUpdateTelegram{update: gateway.DecodedUpdate{Message: test.message}},
				tx:       passthroughTransactionManager{},
			}

			if processErr := implementation.processInboxItem(context.Background(), item, lease); processErr != nil {
				t.Fatal(processErr)
			}
			if repo.deleteCalls != 1 || repo.deletedInboxID != item.ID || repo.deletedLease != lease {
				t.Fatalf(
					"delete calls=%d lease=%+v inbox_id=%d",
					repo.deleteCalls,
					repo.deletedLease,
					repo.deletedInboxID,
				)
			}
			if (repo.externalLookupCalls == 1) != test.wantExternalLookup {
				t.Fatalf("external lookup calls=%d", repo.externalLookupCalls)
			}
			if test.wantExternalLookup && repo.externalID != externalChatID {
				t.Fatalf("external lookup id=%d", repo.externalID)
			}
			if (repo.topicLookupCalls == 1) != test.wantTopicLookup {
				t.Fatalf("topic lookup calls=%d", repo.topicLookupCalls)
			}
			if test.wantTopicLookup && (repo.topicSupergroupID != supergroupID || repo.topicID != topicID) {
				t.Fatalf(
					"topic lookup supergroup=%d topic=%d",
					repo.topicSupergroupID,
					repo.topicID,
				)
			}
			if (repo.createChatCalls == 1) != test.wantCreate {
				t.Fatalf("create chat calls=%d", repo.createChatCalls)
			}
			if test.wantCreate &&
				(repo.createdChat.ExternalChatID != externalChatID || repo.createdChat.SupergroupID != supergroupID) {
				t.Fatalf("created chat=%+v", repo.createdChat)
			}
			if test.wantOperation == nil {
				if repo.updateChatCalls != 0 || repo.createOperationCalls != 0 {
					t.Fatalf("chat updates=%d operation creates=%d", repo.updateChatCalls, repo.createOperationCalls)
				}

				return
			}
			if repo.updateChatCalls != 1 || repo.updatedChat.NextChatOffset != test.wantUpdatedOffset {
				t.Fatalf("chat updates=%d updated chat=%+v", repo.updateChatCalls, repo.updatedChat)
			}
			if repo.createOperationCalls != 1 || repo.operationChatID != test.wantOperationChatID ||
				repo.operationOffset != test.wantOperationOffset ||
				repo.operation.DeduplicationKey != test.wantOperation.DeduplicationKey ||
				repo.operation.Type != test.wantOperation.Type || !reflect.DeepEqual(repo.operation.Payload, test.wantOperation.Payload) {
				t.Fatalf(
					"operation creates=%d chat=%d offset=%d operation=%+v",
					repo.createOperationCalls,
					repo.operationChatID,
					repo.operationOffset,
					repo.operation,
				)
			}
		})
	}
}

func TestProcessInboxFailsMalformedUpdate(t *testing.T) {
	t.Parallel()

	decodeErr := errors.New("malformed update")
	lease := repository.Lease{BotID: 5, Owner: uuid.New(), Version: 7, Offset: 11}
	item := repository.InboxItem{ID: 13, UpdateID: 17}
	repo := &processInboxRepository{}
	metrics := NewMetrics(prometheus.NewRegistry(), "test", "telegram_proxy")
	implementation := &Implementation{
		cfg:      &config.Config{},
		inbox:    repo,
		chats:    repo,
		outbox:   repo,
		telegram: decodedUpdateTelegram{err: decodeErr},
		tx:       passthroughTransactionManager{},
		metrics:  metrics,
	}

	if err := implementation.processInboxItem(context.Background(), item, lease); err != nil {
		t.Fatal(err)
	}
	if repo.failCalls != 1 || repo.failedItem.ID != item.ID || repo.failureMessage != decodeErr.Error() {
		t.Fatalf("fail calls=%d item=%+v message=%q", repo.failCalls, repo.failedItem, repo.failureMessage)
	}
	if repo.deleteCalls != 0 {
		t.Fatalf("delete calls=%d", repo.deleteCalls)
	}
	if value := testutil.ToFloat64(metrics.inboxDispositions.WithLabelValues(inboxDispositionFailed)); value != 1 {
		t.Fatalf("failed inbox disposition metric=%v, want 1", value)
	}
}

func TestProcessInboxRecordsExhaustedFailure(t *testing.T) {
	t.Parallel()

	processingErr := errors.New("database unavailable")
	lease := repository.Lease{BotID: 5, Owner: uuid.New(), Version: 7, Offset: 11}
	item := repository.InboxItem{ID: 13, UpdateID: 17}
	repo := &processInboxRepository{externalLookupErr: processingErr}
	metrics := NewMetrics(prometheus.NewRegistry(), "test", "telegram_proxy")
	implementation := &Implementation{
		cfg: &config.Config{
			Inbox: config.Inbox{Retry: config.Retry{MaxAttempts: 1}},
		},
		inbox:  repo,
		chats:  repo,
		outbox: repo,
		telegram: decodedUpdateTelegram{update: gateway.DecodedUpdate{Message: &gateway.DecodedMessage{
			ID: 19, ChatID: 23, ChatType: "private", SupportedContent: true,
		}}},
		tx:      passthroughTransactionManager{},
		metrics: metrics,
	}

	if err := implementation.processInboxItem(context.Background(), item, lease); err != nil {
		t.Fatal(err)
	}
	if repo.failCalls != 1 || repo.failureMessage != processingErr.Error() {
		t.Fatalf("fail calls=%d message=%q", repo.failCalls, repo.failureMessage)
	}
	if value := testutil.ToFloat64(metrics.inboxDispositions.WithLabelValues(inboxDispositionFailed)); value != 1 {
		t.Fatalf("failed inbox disposition metric=%v, want 1", value)
	}
}

func TestTopicTitleFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message *gateway.DecodedMessage
		want    string
	}{
		{
			name: "username", message: &gateway.DecodedMessage{ChatID: 7, Username: "example", FirstName: "First"},
			want: "@example",
		},
		{
			name: "display name", message: &gateway.DecodedMessage{ChatID: 7, FirstName: "First", LastName: "Last"},
			want: "First Last",
		},
		{
			name: "maximum length", message: &gateway.DecodedMessage{Username: strings.Repeat("a", 200)},
			want: "@" + strings.Repeat("a", 127),
		},
		{
			name: "multibyte maximum length", message: &gateway.DecodedMessage{FirstName: strings.Repeat("Ж", 200)},
			want: strings.Repeat("Ж", 128),
		},
		{name: "user id", message: &gateway.DecodedMessage{ChatID: 7}, want: "User 7"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := topicTitle(test.message)
			if got != test.want {
				t.Fatalf("topicTitle() = %q, want %q", got, test.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("topicTitle() returned invalid UTF-8: %q", got)
			}
		})
	}
}

type processInboxRepository struct {
	repository.Inbox
	repository.Chat
	repository.Outbox

	externalChat         *entity.Chat
	externalLookupErr    error
	externalLookupCalls  int
	externalID           entity.ExternalChatID
	topicChat            *entity.Chat
	topicLookupErr       error
	topicLookupCalls     int
	topicSupergroupID    int64
	topicID              entity.TopicID
	createChatCalls      int
	createdChat          *entity.Chat
	updateChatCalls      int
	updatedChat          *entity.Chat
	createOperationCalls int
	operationChatID      entity.ChatID
	operationOffset      int64
	operation            repository.NewOperation
	deleteCalls          int
	deletedLease         repository.Lease
	deletedInboxID       int64
	failCalls            int
	failedItem           repository.InboxItem
	failureMessage       string
}

type decodedUpdateTelegram struct {
	gateway.Telegram

	update gateway.DecodedUpdate
	err    error
}

func (g decodedUpdateTelegram) DecodeUpdate([]byte) (gateway.DecodedUpdate, error) {
	return g.update, g.err
}

func (r *processInboxRepository) GetChatByExternalID(
	_ context.Context,
	externalChatID entity.ExternalChatID,
) (*entity.Chat, error) {
	r.externalLookupCalls++
	r.externalID = externalChatID

	return r.externalChat, r.externalLookupErr
}

func (r *processInboxRepository) GetChatByTopic(
	_ context.Context,
	supergroupID int64,
	topicID entity.TopicID,
) (*entity.Chat, error) {
	r.topicLookupCalls++
	r.topicSupergroupID = supergroupID
	r.topicID = topicID
	return r.topicChat, r.topicLookupErr
}

func (r *processInboxRepository) CreateChat(_ context.Context, chat *entity.Chat) error {
	r.createChatCalls++
	chat.ID = 41
	r.createdChat = chat

	return nil
}

func (r *processInboxRepository) UpdateChat(_ context.Context, chat *entity.Chat) error {
	r.updateChatCalls++
	r.updatedChat = chat

	return nil
}

func (r *processInboxRepository) CreateOperation(
	_ context.Context,
	req repository.CreateOperationRequest,
) error {
	r.createOperationCalls++
	r.operationChatID = req.ChatID
	r.operationOffset = req.ChatOffset
	r.operation = req.Operation

	return nil
}

func (r *processInboxRepository) FailInbox(
	_ context.Context,
	req repository.FailInboxRequest,
) error {
	r.failCalls++
	r.failedItem = req.Item
	r.failureMessage = req.Message

	return nil
}

func (r *processInboxRepository) DeleteInbox(_ context.Context, lease repository.Lease, inboxID int64) error {
	r.deleteCalls++
	r.deletedLease = lease
	r.deletedInboxID = inboxID

	return nil
}
