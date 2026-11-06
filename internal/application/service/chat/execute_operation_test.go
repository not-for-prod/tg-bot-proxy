package chat //nolint:testpackage // storeTopic transaction behavior is package-private.

import (
	"context"
	"errors"
	"testing"
	"time"

	"tg-bot-proxy/config"
	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/domain/gateway"
	"tg-bot-proxy/internal/domain/repository"

	"github.com/avito-tech/go-transaction-manager/trm/v2"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestExecuteOperationReplacesMissingTopicOnce(t *testing.T) {
	t.Parallel()

	oldTopicID := entity.TopicID(44)
	operation := externalRelayOperation(&oldTopicID)
	repo := &executeOperationRepository{chat: operationChat(operation)}
	telegram := &executeOperationTelegram{
		createdTopicIDs: []int64{55},
		copyErrors: []error{
			&gateway.TelegramError{Code: 400, HTTPStatus: 400, Kind: gateway.TelegramOutcomeMissingTopic},
			nil,
		},
	}
	implementation := executeOperationImplementation(repo, repo, telegram)

	if err := implementation.ExecuteOperation(context.Background(), operation); err != nil {
		t.Fatal(err)
	}
	if telegram.createCalls != 1 {
		t.Fatalf("create calls=%d", telegram.createCalls)
	}
	if len(telegram.copyRequests) != 2 {
		t.Fatalf("copy calls=%d", len(telegram.copyRequests))
	}
	if telegram.copyRequests[0].TopicID == nil || *telegram.copyRequests[0].TopicID != 44 {
		t.Fatalf("initial topic=%v", telegram.copyRequests[0].TopicID)
	}
	if telegram.copyRequests[1].TopicID == nil || *telegram.copyRequests[1].TopicID != 55 {
		t.Fatalf("retry topic=%v", telegram.copyRequests[1].TopicID)
	}
	if telegram.copyRequests[0].SourceChatID != telegram.copyRequests[1].SourceChatID ||
		telegram.copyRequests[0].SourceMessageID != telegram.copyRequests[1].SourceMessageID ||
		telegram.copyRequests[0].DestinationChatID != telegram.copyRequests[1].DestinationChatID {
		t.Fatalf("retry changed copy request: first=%+v second=%+v", telegram.copyRequests[0], telegram.copyRequests[1])
	}
	if repo.completeCalls != 1 || repo.commitOffsetCalls != 1 || repo.retryCalls != 0 || repo.failCalls != 0 {
		t.Fatalf(
			"complete=%d commit offset=%d retry=%d fail=%d",
			repo.completeCalls,
			repo.commitOffsetCalls,
			repo.retryCalls,
			repo.failCalls,
		)
	}
}

//nolint:gocognit // The table keeps both ambiguous creation paths and their disposition assertions together.
func TestExecuteOperationTreatsAmbiguousTopicCreationAsTerminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		topicID    *entity.TopicID
		copyErrors []error
	}{
		{name: "initial creation"},
		{
			name:    "replacement",
			topicID: topicIDPointer(44),
			copyErrors: []error{
				&gateway.TelegramError{Code: 400, HTTPStatus: 400, Kind: gateway.TelegramOutcomeMissingTopic},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			operation := externalRelayOperation(test.topicID)
			repo := &executeOperationRepository{chat: operationChat(operation)}
			telegram := &executeOperationTelegram{
				createErrors: []error{&gateway.TelegramError{
					Code:       500,
					HTTPStatus: 503,
					Kind:       gateway.TelegramOutcomeAmbiguous,
				}},
				copyErrors: test.copyErrors,
			}
			implementation := executeOperationImplementation(repo, repo, telegram)

			if err := implementation.ExecuteOperation(context.Background(), operation); err != nil {
				t.Fatal(err)
			}
			if telegram.createCalls != 1 {
				t.Fatalf("create calls=%d", telegram.createCalls)
			}
			if repo.failCalls != 1 || repo.retryCalls != 0 || repo.completeCalls != 0 {
				t.Fatalf(
					"complete=%d retry=%d fail=%d",
					repo.completeCalls,
					repo.retryCalls,
					repo.failCalls,
				)
			}
			if repo.failure.HTTPStatus == nil || *repo.failure.HTTPStatus != 503 {
				t.Fatalf("failure HTTP status=%v", repo.failure.HTTPStatus)
			}
			if repo.failure.ErrorCode == nil || *repo.failure.ErrorCode != 500 {
				t.Fatalf("failure error code=%v", repo.failure.ErrorCode)
			}
			if value := testutil.ToFloat64(implementation.metrics.outboxDispositions.WithLabelValues(
				string(operation.Type),
				operationDispositionFailed,
			)); value != 1 {
				t.Fatalf("failed disposition metric=%v, want 1", value)
			}
		})
	}
}

func TestExecuteOperationTreatsSecondMissingTopicAsTerminal(t *testing.T) {
	t.Parallel()

	operation := externalRelayOperation(topicIDPointer(44))
	repo := &executeOperationRepository{chat: operationChat(operation)}
	telegram := &executeOperationTelegram{
		createdTopicIDs: []int64{55},
		copyErrors: []error{
			&gateway.TelegramError{Code: 400, HTTPStatus: 400, Kind: gateway.TelegramOutcomeMissingTopic},
			&gateway.TelegramError{Code: 400, HTTPStatus: 400, Kind: gateway.TelegramOutcomeMissingTopic},
		},
	}
	implementation := executeOperationImplementation(repo, repo, telegram)

	if err := implementation.ExecuteOperation(context.Background(), operation); err != nil {
		t.Fatal(err)
	}
	if telegram.createCalls != 1 || len(telegram.copyRequests) != 2 {
		t.Fatalf("create=%d copy=%d", telegram.createCalls, len(telegram.copyRequests))
	}
	if repo.failCalls != 1 || repo.retryCalls != 0 || repo.completeCalls != 0 {
		t.Fatalf(
			"complete=%d retry=%d fail=%d",
			repo.completeCalls,
			repo.retryCalls,
			repo.failCalls,
		)
	}
	if repo.failure.HTTPStatus == nil || *repo.failure.HTTPStatus != 400 {
		t.Fatalf("failure HTTP status=%v", repo.failure.HTTPStatus)
	}
}

func TestExecuteOperationRetriesRetryableOutcome(t *testing.T) {
	t.Parallel()

	operation := entity.Operation{
		ID:             11,
		ChatID:         7,
		Type:           entity.OperationRelayInternalToExternal,
		Payload:        entity.RelayInternalToExternalPayload{SourceChatID: 101, SourceMessageID: 202},
		AttemptCount:   1,
		LeaseOwner:     uuid.New(),
		LeaseVersion:   3,
		ExternalChatID: 101,
	}
	repo := &executeOperationRepository{}
	telegram := &executeOperationTelegram{copyErrors: []error{&gateway.TelegramError{
		Code:        429,
		HTTPStatus:  429,
		RetryAfter:  5,
		Description: "rate limited",
		Kind:        gateway.TelegramOutcomeRetryable,
	}}}
	implementation := executeOperationImplementation(repo, repo, telegram)

	if err := implementation.ExecuteOperation(context.Background(), operation); err != nil {
		t.Fatal(err)
	}
	if repo.retryCalls != 1 || repo.failCalls != 0 || repo.completeCalls != 0 {
		t.Fatalf("complete=%d retry=%d fail=%d", repo.completeCalls, repo.retryCalls, repo.failCalls)
	}
	if value := testutil.ToFloat64(implementation.metrics.outboxDispositions.WithLabelValues(
		string(operation.Type),
		operationDispositionRetry,
	)); value != 1 {
		t.Fatalf("retry disposition metric=%v, want 1", value)
	}
}

func TestStoreTopicUsesOperationLeaseFence(t *testing.T) {
	t.Parallel()

	chat := entity.NewChat(101, -100)
	chat.ID = 7
	operation := entity.Operation{
		ID:           11,
		ChatID:       chat.ID,
		LeaseOwner:   uuid.New(),
		LeaseVersion: 3,
	}
	repo := &storeTopicRepository{chat: chat}
	implementation := &Implementation{chats: repo, outbox: repo, tx: passthroughTransactionManager{}}

	if err := implementation.storeTopic(context.Background(), &operation, 55); err != nil {
		t.Fatal(err)
	}
	if repo.lockedOperation.ID != operation.ID || repo.lockedOperation.LeaseOwner != operation.LeaseOwner ||
		repo.lockedOperation.LeaseVersion != operation.LeaseVersion {
		t.Fatalf("unexpected operation fence: %+v", repo.lockedOperation)
	}
	if operation.TopicID == nil || *operation.TopicID != 55 {
		t.Fatalf("operation topic=%v", operation.TopicID)
	}
	if len(repo.calls) != 3 || repo.calls[0] != "lock operation" || repo.calls[1] != "get chat" ||
		repo.calls[2] != "update chat" {
		t.Fatalf("repository calls=%v", repo.calls)
	}
}

func TestStoreTopicDoesNotUpdateOperationAfterStaleLease(t *testing.T) {
	t.Parallel()

	chat := entity.NewChat(101, -100)
	chat.ID = 7
	operation := entity.Operation{ID: 11, ChatID: chat.ID, LeaseOwner: uuid.New(), LeaseVersion: 3}
	repo := &storeTopicRepository{chat: chat, lockErr: repository.ErrStaleLease}
	implementation := &Implementation{chats: repo, outbox: repo, tx: passthroughTransactionManager{}}

	err := implementation.storeTopic(context.Background(), &operation, 55)
	if !errors.Is(err, repository.ErrStaleLease) {
		t.Fatalf("expected stale lease, got %v", err)
	}
	if operation.TopicID != nil {
		t.Fatalf("stale operation topic=%v", operation.TopicID)
	}
	if len(repo.calls) != 1 || repo.calls[0] != "lock operation" {
		t.Fatalf("repository calls=%v", repo.calls)
	}
}

func TestExecuteOperationCancelsWhenLeaseRenewalIsStale(t *testing.T) {
	t.Parallel()

	repo := &staleRenewalRepository{}
	telegram := &cancellableTelegram{}
	implementation := executeOperationImplementation(repo, repo, telegram)
	implementation.cfg.Outbox.LeaseDuration = 15 * time.Millisecond
	operation := entity.Operation{
		ID:             9,
		Type:           entity.OperationRelayInternalToExternal,
		Payload:        entity.RelayInternalToExternalPayload{SourceChatID: 1, SourceMessageID: 2},
		LeaseOwner:     uuid.New(),
		LeaseVersion:   4,
		ExternalChatID: 10,
	}

	err := implementation.ExecuteOperation(context.Background(), operation)
	if !errors.Is(err, repository.ErrStaleLease) {
		t.Fatalf("expected stale lease, got %v", err)
	}
	if repo.failCalls != 0 {
		t.Fatalf("fail calls=%d, want 0", repo.failCalls)
	}
}

func TestExecuteOperationFinalizesAmbiguousCreationAfterLeaseRenewalError(t *testing.T) {
	t.Parallel()

	repo := &ambiguousLeaseRepository{
		renewErr: context.Canceled,
		renewed:  make(chan struct{}),
	}
	telegram := &ambiguousCancellationTelegram{renewed: repo.renewed}
	implementation := executeOperationImplementation(repo, repo, telegram)
	implementation.cfg.Outbox.LeaseDuration = 15 * time.Millisecond
	operation := externalRelayOperation(nil)

	if err := implementation.ExecuteOperation(context.Background(), operation); err != nil {
		t.Fatal(err)
	}
	if repo.failCalls != 1 {
		t.Fatalf("fail calls=%d, want 1", repo.failCalls)
	}
	if repo.failContextErr != nil {
		t.Fatalf("finalization context error=%v", repo.failContextErr)
	}
	if repo.failure.ErrorCode == nil || *repo.failure.ErrorCode != 500 {
		t.Fatalf("failure error code=%v", repo.failure.ErrorCode)
	}
}

func TestExecuteOperationFinalizesAmbiguousCreationAfterParentCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	repo := &ambiguousLeaseRepository{parentCancel: cancel}
	implementation := executeOperationImplementation(repo, repo, &immediateAmbiguousTelegram{})
	implementation.cfg.Outbox.LeaseDuration = time.Hour

	if err := implementation.ExecuteOperation(ctx, externalRelayOperation(nil)); err != nil {
		t.Fatal(err)
	}
	if repo.failCalls != 1 {
		t.Fatalf("fail calls=%d, want 1", repo.failCalls)
	}
	if repo.failContextErr != nil {
		t.Fatalf("finalization context error=%v", repo.failContextErr)
	}
}

func TestExecuteOperationDoesNotFinalizeAmbiguousCreationAfterStaleLease(t *testing.T) {
	t.Parallel()

	repo := &ambiguousLeaseRepository{
		renewErr: repository.ErrStaleLease,
		renewed:  make(chan struct{}),
	}
	telegram := &ambiguousCancellationTelegram{renewed: repo.renewed}
	implementation := executeOperationImplementation(repo, repo, telegram)
	implementation.cfg.Outbox.LeaseDuration = 15 * time.Millisecond
	operation := externalRelayOperation(nil)

	err := implementation.ExecuteOperation(context.Background(), operation)
	if !errors.Is(err, repository.ErrStaleLease) {
		t.Fatalf("expected stale lease, got %v", err)
	}
	if repo.failCalls != 0 {
		t.Fatalf("fail calls=%d, want 0", repo.failCalls)
	}
}

type ambiguousLeaseRepository struct {
	repository.Outbox
	repository.Chat

	renewErr       error
	renewed        chan struct{}
	parentCancel   context.CancelFunc
	failCalls      int
	failContextErr error
	failure        repository.Failure
}

func (r *ambiguousLeaseRepository) RenewOperation(context.Context, entity.Operation, time.Time) error {
	close(r.renewed)

	return r.renewErr
}

func (r *ambiguousLeaseRepository) FailOperation(
	ctx context.Context,
	_ entity.Operation,
	failure repository.Failure,
) error {
	if r.parentCancel != nil {
		r.parentCancel()
	}
	r.failCalls++
	r.failContextErr = ctx.Err()
	r.failure = failure

	return nil
}

type immediateAmbiguousTelegram struct {
	gateway.Telegram
}

func (g *immediateAmbiguousTelegram) CreateForumTopic(context.Context, string) (int64, error) {
	return 0, &gateway.TelegramError{Code: 500, HTTPStatus: 503, Kind: gateway.TelegramOutcomeAmbiguous}
}

type ambiguousCancellationTelegram struct {
	gateway.Telegram

	renewed <-chan struct{}
}

func (g *ambiguousCancellationTelegram) CreateForumTopic(ctx context.Context, _ string) (int64, error) {
	select {
	case <-g.renewed:
	case <-ctx.Done():
	}
	<-ctx.Done()

	return 0, &gateway.TelegramError{Code: 500, HTTPStatus: 503, Kind: gateway.TelegramOutcomeAmbiguous}
}

type staleRenewalRepository struct {
	repository.Outbox
	repository.Chat

	failCalls int
}

func (r *staleRenewalRepository) RenewOperation(context.Context, entity.Operation, time.Time) error {
	return repository.ErrStaleLease
}

func (r *staleRenewalRepository) FailOperation(
	context.Context,
	entity.Operation,
	repository.Failure,
) error {
	r.failCalls++
	return nil
}

type cancellableTelegram struct {
	gateway.Telegram
}

func (g *cancellableTelegram) CopyMessage(ctx context.Context, _ gateway.CopyMessageRequest) error {
	<-ctx.Done()
	return ctx.Err()
}

type storeTopicRepository struct {
	repository.Outbox
	repository.Chat

	chat            *entity.Chat
	lockedOperation entity.Operation
	lockErr         error
	calls           []string
}

type executeOperationRepository struct {
	repository.Outbox
	repository.Chat

	chat              *entity.Chat
	completeCalls     int
	commitOffsetCalls int
	retryCalls        int
	failCalls         int
	failure           repository.Failure
}

func (r *executeOperationRepository) GetChatByExternalID(
	context.Context,
	entity.ExternalChatID,
) (*entity.Chat, error) {
	return r.chat, nil
}

func (r *executeOperationRepository) LockOperation(context.Context, entity.Operation) error {
	return nil
}

func (r *executeOperationRepository) UpdateChat(context.Context, *entity.Chat) error {
	return nil
}

func (r *executeOperationRepository) CompleteOperation(context.Context, entity.Operation) error {
	r.completeCalls++

	return nil
}

func (r *executeOperationRepository) CommitChatOffset(context.Context, entity.ChatID, int64) error {
	r.commitOffsetCalls++

	return nil
}

func (r *executeOperationRepository) RetryOperation(
	context.Context,
	repository.RetryOperationRequest,
) error {
	r.retryCalls++

	return nil
}

func (r *executeOperationRepository) FailOperation(
	_ context.Context,
	_ entity.Operation,
	failure repository.Failure,
) error {
	r.failCalls++
	r.failure = failure

	return nil
}

type executeOperationTelegram struct {
	gateway.Telegram

	createdTopicIDs []int64
	createErrors    []error
	copyErrors      []error
	createCalls     int
	copyRequests    []gateway.CopyMessageRequest
}

func (g *executeOperationTelegram) CreateForumTopic(context.Context, string) (int64, error) {
	call := g.createCalls
	g.createCalls++

	var topicID int64
	if call < len(g.createdTopicIDs) {
		topicID = g.createdTopicIDs[call]
	}
	if call < len(g.createErrors) {
		return topicID, g.createErrors[call]
	}

	return topicID, nil
}

func (g *executeOperationTelegram) CopyMessage(_ context.Context, request gateway.CopyMessageRequest) error {
	g.copyRequests = append(g.copyRequests, request)
	call := len(g.copyRequests) - 1
	if call < len(g.copyErrors) {
		return g.copyErrors[call]
	}

	return nil
}

func executeOperationImplementation(
	outbox repository.Outbox,
	chats repository.Chat,
	telegram gateway.Telegram,
) *Implementation {
	return &Implementation{
		cfg: &config.Config{
			Outbox: config.Outbox{
				LeaseDuration: time.Minute,
				Retry: config.Retry{
					MaxAttempts:  3,
					InitialDelay: time.Second,
					MaxDelay:     time.Minute,
					Multiplier:   2,
				},
			},
		},
		outbox:   outbox,
		chats:    chats,
		telegram: telegram,
		tx:       passthroughTransactionManager{},
		metrics:  NewMetrics(prometheus.NewRegistry(), "test", "telegram_proxy"),
	}
}

func externalRelayOperation(topicID *entity.TopicID) entity.Operation {
	return entity.Operation{
		ID:     11,
		ChatID: 7,
		Type:   entity.OperationRelayExternalToInternal,
		Payload: entity.RelayExternalToInternalPayload{
			SourceChatID: 101, SourceMessageID: 202, TopicTitle: "Support",
		},
		AttemptCount:   1,
		LeaseOwner:     uuid.New(),
		LeaseVersion:   3,
		ExternalChatID: 101,
		SupergroupID:   -100,
		TopicID:        topicID,
	}
}

func operationChat(operation entity.Operation) *entity.Chat {
	chat := entity.NewChat(operation.ExternalChatID, operation.SupergroupID)
	chat.ID = operation.ChatID

	return chat
}

func topicIDPointer(value entity.TopicID) *entity.TopicID {
	return &value
}

func (r *storeTopicRepository) GetChatByExternalID(
	context.Context,
	entity.ExternalChatID,
) (*entity.Chat, error) {
	r.calls = append(r.calls, "get chat")
	return r.chat, nil
}

func (r *storeTopicRepository) LockOperation(_ context.Context, operation entity.Operation) error {
	r.calls = append(r.calls, "lock operation")
	r.lockedOperation = operation
	return r.lockErr
}

func (r *storeTopicRepository) UpdateChat(context.Context, *entity.Chat) error {
	r.calls = append(r.calls, "update chat")
	return nil
}

type passthroughTransactionManager struct{}

func (passthroughTransactionManager) Do(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func (passthroughTransactionManager) DoWithSettings(
	ctx context.Context,
	_ trm.Settings,
	fn func(context.Context) error,
) error {
	return fn(ctx)
}
