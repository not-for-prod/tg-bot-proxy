package repositoryintegration_test //nolint:cyclop // Database lifecycle scenarios are intentionally explicit.

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/domain/gateway"
	"tg-bot-proxy/internal/domain/repository"

	txpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type integrationFixture struct {
	ctx  context.Context
	dsn  string
	pool *pgxpool.Pool
	repo *Implementation
	tx   trm.Manager
}

const resetTablesSQL = "TRUNCATE outbox, inbox, chats, inbox_processor_lock, telegram_bot_offset RESTART IDENTITY"

func newIntegrationFixture(t *testing.T) *integrationFixture {
	t.Helper()

	dsn := os.Getenv("TELEGRAM_PROXY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TELEGRAM_PROXY_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err = pool.Exec(ctx, resetTablesSQL); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, cleanupErr := pool.Exec(ctx, resetTablesSQL); cleanupErr != nil {
			t.Errorf("reset integration database: %v", cleanupErr)
		}
	})

	return &integrationFixture{
		ctx:  ctx,
		dsn:  dsn,
		pool: pool,
		repo: NewImplementation(pool, txpgx.DefaultCtxGetter),
		tx:   manager.Must(txpgx.NewDefaultFactory(pool)),
	}
}

func TestPollingLeasePersistenceAndFencing(t *testing.T) {
	fixture := newIntegrationFixture(t)
	ctx, pool, repo, tx := fixture.ctx, fixture.pool, fixture.repo, fixture.tx
	var err error
	owner := uuid.New()
	lease, claimed, err := repo.ClaimIngestion(ctx, repository.ClaimLeaseRequest{
		BotID: 10, Owner: owner, LeaseUntil: time.Now().Add(time.Minute),
	})
	if err != nil || !claimed {
		t.Fatalf("claim failed: claimed=%v err=%v", claimed, err)
	}
	updates := []gateway.TelegramUpdate{
		{ID: 4, Payload: []byte(`{"update_id":4}`)},
		{ID: 4, Payload: []byte(`{"update_id":4}`)},
	}
	if err = tx.Do(ctx, func(ctx context.Context) error {
		if createErr := repo.CreateInboxItems(ctx, repository.CreateInboxItemsRequest{
			BotID: lease.BotID, Updates: updates,
		}); createErr != nil {
			return createErr
		}

		return repo.CommitIngestion(ctx, repository.CommitIngestionRequest{Lease: *lease, NextOffset: 5})
	}); err != nil {
		t.Fatal(err)
	}

	var offset, inboxCount int64
	if err = pool.QueryRow(ctx, "SELECT update_id FROM telegram_bot_offset WHERE bot_id=10").Scan(&offset); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM inbox WHERE bot_id=10").Scan(&inboxCount); err != nil {
		t.Fatal(err)
	}
	if offset != 5 || inboxCount != 1 {
		t.Fatalf("offset=%d inboxCount=%d", offset, inboxCount)
	}
	if err = repo.ReleaseIngestion(ctx, *lease); !errors.Is(err, repository.ErrStaleLease) {
		t.Fatalf("expected stale lease, got %v", err)
	}
}

func TestCommitIngestionRollsBackWhenExpiredLeaseWasReclaimed(t *testing.T) {
	fixture := newIntegrationFixture(t)
	ctx, pool, repo, tx := fixture.ctx, fixture.pool, fixture.repo, fixture.tx
	var err error
	now := time.Now().UTC()
	staleLease, claimed, err := repo.ClaimIngestion(ctx, repository.ClaimLeaseRequest{
		BotID: 10, Owner: uuid.New(), LeaseUntil: now.Add(-time.Minute),
	})
	if err != nil || !claimed {
		t.Fatalf("claim expired lease: claimed=%v err=%v", claimed, err)
	}
	currentLease, claimed, err := repo.ClaimIngestion(ctx, repository.ClaimLeaseRequest{
		BotID: 10, Owner: uuid.New(), LeaseUntil: now.Add(time.Minute),
	})
	if err != nil || !claimed {
		t.Fatalf("reclaim lease: claimed=%v err=%v", claimed, err)
	}
	var leaseUntilBefore time.Time
	if err = pool.QueryRow(ctx, "SELECT lease_until FROM telegram_bot_offset WHERE bot_id=$1", currentLease.BotID).
		Scan(&leaseUntilBefore); err != nil {
		t.Fatal(err)
	}

	updates := []gateway.TelegramUpdate{{ID: 7, Payload: []byte(`{"update_id":7}`)}}
	err = tx.Do(ctx, func(ctx context.Context) error {
		if createErr := repo.CreateInboxItems(ctx, repository.CreateInboxItemsRequest{
			BotID: staleLease.BotID, Updates: updates,
		}); createErr != nil {
			return createErr
		}

		return repo.CommitIngestion(ctx, repository.CommitIngestionRequest{Lease: *staleLease, NextOffset: 8})
	})
	if !errors.Is(err, repository.ErrStaleLease) {
		t.Fatalf("expected stale lease, got %v", err)
	}

	var offset, version, inboxCount int64
	var owner uuid.UUID
	var leaseUntil time.Time
	if err = pool.QueryRow(ctx, `SELECT update_id, lease_owner, lease_until, lease_version
FROM telegram_bot_offset WHERE bot_id=$1`, currentLease.BotID).
		Scan(&offset, &owner, &leaseUntil, &version); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM inbox WHERE bot_id=$1", currentLease.BotID).Scan(&inboxCount); err != nil {
		t.Fatal(err)
	}
	leaseUnchanged := offset == currentLease.Offset && owner == currentLease.Owner &&
		leaseUntil.Equal(leaseUntilBefore) && version == currentLease.Version
	if !leaseUnchanged || inboxCount != 0 {
		t.Fatalf(
			"offset=%d owner=%s version=%d inboxCount=%d currentLease=%+v",
			offset,
			owner,
			version,
			inboxCount,
			currentLease,
		)
	}
}

//nolint:gocognit // The cases share one real lease lifecycle and database fixture.
func TestInboxMutationsRejectStaleLease(t *testing.T) {
	fixture := newIntegrationFixture(t)
	ctx, pool, repo, tx := fixture.ctx, fixture.pool, fixture.repo, fixture.tx
	var err error
	staleLease, claimed, err := repo.ClaimInboxProcessor(ctx, repository.ClaimLeaseRequest{
		BotID: 10, Owner: uuid.New(), LeaseUntil: time.Now().Add(-time.Minute),
	})
	if err != nil || !claimed {
		t.Fatalf("claim stale lease: claimed=%v err=%v", claimed, err)
	}
	currentLease, claimed, err := repo.ClaimInboxProcessor(ctx, repository.ClaimLeaseRequest{
		BotID: 10, Owner: uuid.New(), LeaseUntil: time.Now().Add(time.Minute),
	})
	if err != nil || !claimed {
		t.Fatalf("reclaim lease: claimed=%v err=%v", claimed, err)
	}

	items := make([]repository.InboxItem, 4)
	for index := range items {
		updateID := int64(index + 1)
		err = pool.QueryRow(ctx, `INSERT INTO inbox (bot_id,update_id,payload) VALUES ($1,$2,$3) RETURNING id`,
			10,
			updateID,
			[]byte(`{"update_id":`+strconv.FormatInt(updateID, 10)+`}`),
		).Scan(&items[index].ID)
		if err != nil {
			t.Fatal(err)
		}
		items[index].BotID = 10
	}

	if err = repo.DeleteInbox(ctx, *staleLease, items[0].ID); !errors.Is(err, repository.ErrStaleLease) {
		t.Fatalf("stale delete: %v", err)
	}
	err = repo.RetryInbox(ctx, repository.RetryInboxRequest{
		Lease: *staleLease, Item: items[1], AvailableAt: time.Now(), Message: "retry",
	})
	if !errors.Is(err, repository.ErrStaleLease) {
		t.Fatalf("stale retry: %v", err)
	}
	if err = repo.FailInbox(ctx, repository.FailInboxRequest{
		Lease: *staleLease, Item: items[2], Message: "failed",
	}); !errors.Is(err, repository.ErrStaleLease) {
		t.Fatalf("stale fail: %v", err)
	}

	if err = repo.DeleteInbox(ctx, *currentLease, items[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = repo.RetryInbox(ctx, repository.RetryInboxRequest{
		Lease: *currentLease, Item: items[1], AvailableAt: time.Now(), Message: "retry",
	}); err != nil {
		t.Fatal(err)
	}
	if err = repo.FailInbox(ctx, repository.FailInboxRequest{
		Lease: *currentLease, Item: items[2], Message: "failed",
	}); err != nil {
		t.Fatal(err)
	}

	chat := entity.NewChat(500, -100)
	if err = repo.CreateChat(ctx, chat); err != nil {
		t.Fatal(err)
	}
	offset := chat.AllocateOffset()
	err = tx.Do(ctx, func(ctx context.Context) error {
		if updateErr := repo.UpdateChat(ctx, chat); updateErr != nil {
			return updateErr
		}
		if createErr := repo.CreateOperation(ctx, repository.CreateOperationRequest{
			ChatID: chat.ID, ChatOffset: offset,
			Operation: repository.NewOperation{
				DeduplicationKey: "stale-inbox-transaction",
				Type:             entity.OperationSendExternalMessage,
				Payload:          entity.SendExternalMessagePayload{Text: "hello"},
			},
		}); createErr != nil {
			return createErr
		}
		return repo.DeleteInbox(ctx, *staleLease, items[3].ID)
	})
	if !errors.Is(err, repository.ErrStaleLease) {
		t.Fatalf("stale transactional delete: %v", err)
	}

	var outboxCount, inboxCount, failedCount, retryCount, nextOffset int64
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM outbox").Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM inbox").Scan(&inboxCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM inbox WHERE status='failed'").Scan(&failedCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM inbox WHERE status='retry'").Scan(&retryCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT next_chat_offset FROM chats WHERE id=$1", chat.ID).Scan(&nextOffset); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 0 || inboxCount != 3 || failedCount != 1 || retryCount != 1 || nextOffset != 1 {
		t.Fatalf(
			"outbox=%d inbox=%d failed=%d retry=%d nextOffset=%d",
			outboxCount,
			inboxCount,
			failedCount,
			retryCount,
			nextOffset,
		)
	}
}

func TestChatOperationOrderingLifecycle(t *testing.T) {
	fixture := newIntegrationFixture(t)
	ctx, pool, repo, tx := fixture.ctx, fixture.pool, fixture.repo, fixture.tx
	var err error
	chat := entity.NewChat(99, -100)
	if err = repo.CreateChat(ctx, chat); err != nil {
		t.Fatal(err)
	}
	offset := chat.AllocateOffset()
	if err = tx.Do(ctx, func(ctx context.Context) error {
		if updateErr := repo.UpdateChat(ctx, chat); updateErr != nil {
			return updateErr
		}
		return repo.CreateOperation(ctx, repository.CreateOperationRequest{
			ChatID: chat.ID, ChatOffset: offset,
			Operation: repository.NewOperation{
				DeduplicationKey: "external:1",
				Type:             entity.OperationSendExternalMessage,
				Payload:          entity.SendExternalMessagePayload{Text: "hello"},
			},
		})
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	operations, err := repo.ClaimOperations(ctx, repository.ClaimOperationsRequest{
		Limit:      10,
		Owner:      uuid.New(),
		Now:        now,
		LeaseUntil: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 1 || operations[0].ChatOffset != 1 {
		t.Fatalf("unexpected claimed operations: %+v", operations)
	}
	staleOperation := operations[0]
	staleOperation.LeaseVersion--
	err = repo.RenewOperation(ctx, staleOperation, now.Add(2*time.Minute))
	if !errors.Is(err, repository.ErrStaleLease) {
		t.Fatalf("expected stale renewal, got %v", err)
	}
	if err = repo.RenewOperation(ctx, operations[0], now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Do(ctx, func(ctx context.Context) error {
		if completeErr := repo.CompleteOperation(ctx, operations[0]); completeErr != nil {
			return completeErr
		}

		return repo.CommitChatOffset(ctx, operations[0].ChatID, operations[0].ChatOffset)
	}); err != nil {
		t.Fatal(err)
	}

	var committedOffset, operationCount int64
	if err = pool.QueryRow(ctx, "SELECT committed_chat_offset FROM chats WHERE id=$1", chat.ID).Scan(&committedOffset); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM outbox WHERE chat_id=$1", chat.ID).Scan(&operationCount); err != nil {
		t.Fatal(err)
	}
	if committedOffset != 1 || operationCount != 0 {
		t.Fatalf("committedOffset=%d operationCount=%d", committedOffset, operationCount)
	}
}

//nolint:cyclop,gocognit,gocyclo // One scenario makes the cross-chat ordering transitions explicit.
func TestOutboxStateMachineAcrossChats(t *testing.T) {
	fixture := newIntegrationFixture(t)
	ctx, pool, repo, tx := fixture.ctx, fixture.pool, fixture.repo, fixture.tx
	var err error
	chats := []*entity.Chat{entity.NewChat(101, -100), entity.NewChat(202, -100)}
	for _, chat := range chats {
		if err = repo.CreateChat(ctx, chat); err != nil {
			t.Fatal(err)
		}
		if err = tx.Do(ctx, func(ctx context.Context) error {
			for offset := int64(1); offset <= 3; offset++ {
				allocatedOffset := chat.AllocateOffset()
				if createErr := repo.CreateOperation(ctx, repository.CreateOperationRequest{
					ChatID: chat.ID, ChatOffset: allocatedOffset,
					Operation: repository.NewOperation{
						DeduplicationKey: "chat:" + strconv.FormatInt(int64(chat.ID), 10) + ":" + strconv.FormatInt(offset, 10),
						Type:             entity.OperationSendExternalMessage,
						Payload:          entity.SendExternalMessagePayload{Text: "hello"},
					},
				}); createErr != nil {
					return createErr
				}
			}
			return repo.UpdateChat(ctx, chat)
		}); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now().UTC()
	owner := uuid.New()
	claimed, err := repo.ClaimOperations(ctx, repository.ClaimOperationsRequest{
		Limit:      10,
		Owner:      owner,
		Now:        now,
		LeaseUntil: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 2 {
		t.Fatalf("initial claims=%d, want one head from each chat", len(claimed))
	}
	claimedByChat := make(map[entity.ChatID]entity.Operation, len(claimed))
	for _, operation := range claimed {
		if operation.ChatOffset != 1 {
			t.Fatalf("claimed non-head operation: %+v", operation)
		}
		claimedByChat[operation.ChatID] = operation
	}
	firstChatHead, firstFound := claimedByChat[chats[0].ID]
	secondChatHead, secondFound := claimedByChat[chats[1].ID]
	if !firstFound || !secondFound {
		t.Fatalf("claims do not cover both chats: %+v", claimed)
	}

	retryAt := now.Add(time.Hour)
	if err = repo.RetryOperation(ctx, repository.RetryOperationRequest{
		Operation: firstChatHead, AvailableAt: retryAt, Failure: repository.Failure{Message: "temporary"},
	}); err != nil {
		t.Fatal(err)
	}
	if err = repo.FailOperation(ctx, secondChatHead, repository.Failure{Message: "permanent"}); err != nil {
		t.Fatal(err)
	}

	claimed, err = repo.ClaimOperations(ctx, repository.ClaimOperationsRequest{
		Limit:      10,
		Owner:      uuid.New(),
		Now:        now.Add(time.Minute),
		LeaseUntil: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 0 {
		t.Fatalf("retry-unavailable and failed heads must block their chats: %+v", claimed)
	}

	currentClaims, err := repo.ClaimOperations(ctx, repository.ClaimOperationsRequest{
		Limit:      10,
		Owner:      uuid.New(),
		Now:        retryAt,
		LeaseUntil: retryAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(currentClaims) != 1 || currentClaims[0].ChatID != chats[0].ID || currentClaims[0].ChatOffset != 1 {
		t.Fatalf("retry claim=%+v", currentClaims)
	}
	currentHead := currentClaims[0]

	err = tx.Do(ctx, func(ctx context.Context) error {
		if completeErr := repo.CompleteOperation(ctx, firstChatHead); completeErr != nil {
			return completeErr
		}

		return repo.CommitChatOffset(ctx, firstChatHead.ChatID, firstChatHead.ChatOffset)
	})
	if !errors.Is(err, repository.ErrStaleLease) {
		t.Fatalf("stale complete: %v", err)
	}
	var committedOffset, operationCount int64
	if err = pool.QueryRow(ctx, "SELECT committed_chat_offset FROM chats WHERE id=$1", chats[0].ID).Scan(&committedOffset); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM outbox WHERE chat_id=$1", chats[0].ID).Scan(&operationCount); err != nil {
		t.Fatal(err)
	}
	if committedOffset != 0 || operationCount != 3 {
		t.Fatalf("stale complete committed=%d operations=%d", committedOffset, operationCount)
	}

	if err = tx.Do(ctx, func(ctx context.Context) error {
		if completeErr := repo.CompleteOperation(ctx, currentHead); completeErr != nil {
			return completeErr
		}

		return repo.CommitChatOffset(ctx, currentHead.ChatID, currentHead.ChatOffset)
	}); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT committed_chat_offset FROM chats WHERE id=$1", chats[0].ID).Scan(&committedOffset); err != nil {
		t.Fatal(err)
	}
	if committedOffset != 1 {
		t.Fatalf("committed offset=%d, want 1", committedOffset)
	}

	nextClaims, err := repo.ClaimOperations(ctx, repository.ClaimOperationsRequest{
		Limit:      10,
		Owner:      uuid.New(),
		Now:        retryAt,
		LeaseUntil: retryAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(nextClaims) != 1 || nextClaims[0].ChatID != chats[0].ID || nextClaims[0].ChatOffset != 2 {
		t.Fatalf("next claims=%+v", nextClaims)
	}
}

//nolint:gocognit // Exercises both stale and current claims.
func TestLockOperationRejectsStaleLease(t *testing.T) {
	fixture := newIntegrationFixture(t)
	ctx, pool, repo, tx := fixture.ctx, fixture.pool, fixture.repo, fixture.tx
	var err error
	chat := entity.NewChat(entity.ExternalChatID(time.Now().UnixNano()), -100)
	if err = repo.CreateChat(ctx, chat); err != nil {
		t.Fatal(err)
	}
	offset := chat.AllocateOffset()
	if err = tx.Do(ctx, func(ctx context.Context) error {
		if updateErr := repo.UpdateChat(ctx, chat); updateErr != nil {
			return updateErr
		}
		return repo.CreateOperation(ctx, repository.CreateOperationRequest{
			ChatID: chat.ID, ChatOffset: offset,
			Operation: repository.NewOperation{
				DeduplicationKey: "external:" + uuid.NewString(),
				Type:             entity.OperationRelayExternalToInternal,
				Payload: entity.RelayExternalToInternalPayload{
					SourceChatID: 101, SourceMessageID: 1, TopicTitle: "topic",
				},
			},
		})
	}); err != nil {
		t.Fatal(err)
	}

	claimTime := time.Now().UTC()
	firstClaim, err := repo.ClaimOperations(ctx, repository.ClaimOperationsRequest{
		Limit:      1,
		Owner:      uuid.New(),
		Now:        claimTime,
		LeaseUntil: claimTime.Add(time.Minute),
	})
	if err != nil || len(firstClaim) != 1 {
		t.Fatalf("first claim: operations=%d err=%v", len(firstClaim), err)
	}
	secondClaim, err := repo.ClaimOperations(
		ctx,
		repository.ClaimOperationsRequest{
			Limit:      1,
			Owner:      uuid.New(),
			Now:        claimTime.Add(2 * time.Minute),
			LeaseUntil: claimTime.Add(3 * time.Minute),
		},
	)
	if err != nil || len(secondClaim) != 1 {
		t.Fatalf("second claim: operations=%d err=%v", len(secondClaim), err)
	}

	err = tx.Do(ctx, func(ctx context.Context) error {
		if lockErr := repo.LockOperation(ctx, firstClaim[0]); lockErr != nil {
			return lockErr
		}

		loadedChat, getErr := repo.GetChatByExternalID(ctx, chat.ExternalChatID)
		if getErr != nil {
			return getErr
		}
		if mapErr := loadedChat.MapTopic(111); mapErr != nil {
			return mapErr
		}
		return repo.UpdateChat(ctx, loadedChat)
	})
	if !errors.Is(err, repository.ErrStaleLease) {
		t.Fatalf("expected stale lease, got %v", err)
	}

	var topicID *int64
	if err = pool.QueryRow(ctx, "SELECT topic_id FROM chats WHERE id=$1", chat.ID).Scan(&topicID); err != nil {
		t.Fatal(err)
	}
	if topicID != nil {
		t.Fatalf("stale worker stored topic %d", *topicID)
	}

	if err = tx.Do(ctx, func(ctx context.Context) error {
		if lockErr := repo.LockOperation(ctx, secondClaim[0]); lockErr != nil {
			return lockErr
		}

		loadedChat, getErr := repo.GetChatByExternalID(ctx, chat.ExternalChatID)
		if getErr != nil {
			return getErr
		}
		if mapErr := loadedChat.MapTopic(222); mapErr != nil {
			return mapErr
		}
		return repo.UpdateChat(ctx, loadedChat)
	}); err != nil {
		t.Fatal(err)
	}

	if err = pool.QueryRow(ctx, "SELECT topic_id FROM chats WHERE id=$1", chat.ID).Scan(&topicID); err != nil {
		t.Fatal(err)
	}
	if topicID == nil || *topicID != 222 {
		t.Fatalf("current worker topic=%v", topicID)
	}
}

//nolint:gocognit // The complete two-connection scenario is easier to verify in one test.
func TestOperationLockPreventsConcurrentReclaimDuringTopicPersistence(t *testing.T) {
	fixture := newIntegrationFixture(t)
	ctx, workerRepo, tx := fixture.ctx, fixture.repo, fixture.tx
	reclaimerPool, err := pgxpool.New(ctx, fixture.dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reclaimerPool.Close)

	reclaimerRepo := NewImplementation(reclaimerPool, txpgx.DefaultCtxGetter)
	chat := entity.NewChat(entity.ExternalChatID(time.Now().UnixNano()), -100)
	if err = workerRepo.CreateChat(ctx, chat); err != nil {
		t.Fatal(err)
	}
	offset := chat.AllocateOffset()
	if err = tx.Do(ctx, func(ctx context.Context) error {
		if updateErr := workerRepo.UpdateChat(ctx, chat); updateErr != nil {
			return updateErr
		}

		return workerRepo.CreateOperation(ctx, repository.CreateOperationRequest{
			ChatID: chat.ID, ChatOffset: offset,
			Operation: repository.NewOperation{
				DeduplicationKey: "external:" + uuid.NewString(),
				Type:             entity.OperationRelayExternalToInternal,
				Payload: entity.RelayExternalToInternalPayload{
					SourceChatID: 101, SourceMessageID: 1, TopicTitle: "topic",
				},
			},
		})
	}); err != nil {
		t.Fatal(err)
	}

	claimTime := time.Now().UTC()
	claim, err := workerRepo.ClaimOperations(ctx, repository.ClaimOperationsRequest{
		Limit:      1,
		Owner:      uuid.New(),
		Now:        claimTime,
		LeaseUntil: claimTime.Add(time.Minute),
	})
	if err != nil || len(claim) != 1 {
		t.Fatalf("claim: operations=%d err=%v", len(claim), err)
	}

	operationLocked := make(chan struct{})
	persistTopic := make(chan struct{})
	persisted := make(chan error, 1)
	go func() {
		persisted <- tx.Do(ctx, func(ctx context.Context) error {
			if lockErr := workerRepo.LockOperation(ctx, claim[0]); lockErr != nil {
				return lockErr
			}
			close(operationLocked)
			<-persistTopic

			loadedChat, getErr := workerRepo.GetChatByExternalID(ctx, chat.ExternalChatID)
			if getErr != nil {
				return getErr
			}
			if mapErr := loadedChat.MapTopic(333); mapErr != nil {
				return mapErr
			}

			return workerRepo.UpdateChat(ctx, loadedChat)
		})
	}()
	<-operationLocked

	reclaimed, err := reclaimerRepo.ClaimOperations(
		ctx,
		repository.ClaimOperationsRequest{
			Limit:      1,
			Owner:      uuid.New(),
			Now:        claimTime.Add(2 * time.Minute),
			LeaseUntil: claimTime.Add(3 * time.Minute),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(reclaimed) != 0 {
		t.Fatalf("reclaimed locked operation: %+v", reclaimed[0])
	}

	close(persistTopic)
	if err = <-persisted; err != nil {
		t.Fatal(err)
	}

	reclaimed, err = reclaimerRepo.ClaimOperations(
		ctx,
		repository.ClaimOperationsRequest{
			Limit:      1,
			Owner:      uuid.New(),
			Now:        claimTime.Add(2 * time.Minute),
			LeaseUntil: claimTime.Add(3 * time.Minute),
		},
	)
	if err != nil || len(reclaimed) != 1 {
		t.Fatalf("reclaim after commit: operations=%d err=%v", len(reclaimed), err)
	}
	if reclaimed[0].TopicID == nil || *reclaimed[0].TopicID != 333 {
		t.Fatalf("reclaimed topic=%v", reclaimed[0].TopicID)
	}
}
