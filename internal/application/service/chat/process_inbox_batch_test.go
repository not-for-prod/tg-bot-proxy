package chat //nolint:testpackage // The test verifies orchestration through repository calls.

import (
	"context"
	"errors"
	"testing"
	"time"

	"tg-bot-proxy/config"
	"tg-bot-proxy/internal/domain/gateway"
	"tg-bot-proxy/internal/domain/repository"

	"github.com/avito-tech/go-transaction-manager/trm/v2"
	"github.com/google/uuid"
)

func TestProcessInboxBatchDoesNothingWithoutLease(t *testing.T) {
	t.Parallel()

	repo := &processInboxBatchRepository{}
	implementation := newProcessInboxBatchImplementation(repo)

	if err := implementation.ProcessInboxBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.listCalls != 0 || repo.renewCalls != 0 || repo.releaseCalls != 0 {
		t.Fatalf("list=%d renew=%d release=%d", repo.listCalls, repo.renewCalls, repo.releaseCalls)
	}
}

func TestProcessInboxBatchReleasesEmptyClaimedBatch(t *testing.T) {
	t.Parallel()

	lease := repository.Lease{BotID: 11, Owner: uuid.New(), Version: 4}
	repo := &processInboxBatchRepository{
		lease:   lease,
		claimed: true,
	}
	implementation := newProcessInboxBatchImplementation(repo)

	if err := implementation.ProcessInboxBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.listCalls != 1 || repo.renewCalls != 0 || repo.releaseCalls != 1 {
		t.Fatalf("list=%d renew=%d release=%d", repo.listCalls, repo.renewCalls, repo.releaseCalls)
	}
	if repo.releasedLease != lease {
		t.Fatalf("released lease = %+v, want %+v", repo.releasedLease, lease)
	}
}

func TestProcessInboxBatchContinuesAfterIndependentFailureAndReleasesLease(t *testing.T) {
	t.Parallel()

	lease := repository.Lease{BotID: 11, Owner: uuid.New(), Version: 4}
	items := []repository.InboxItem{
		{ID: 1, BotID: 11, UpdateID: 10, Payload: []byte(`{}`)},
		{ID: 2, BotID: 11, UpdateID: 11, Payload: []byte(`{}`)},
	}
	processingErr := errors.New("temporary processing error")
	releaseErr := errors.New("release failed")
	repo := &processInboxBatchRepository{
		lease:      lease,
		claimed:    true,
		items:      items,
		retryErr:   processingErr,
		releaseErr: releaseErr,
	}
	implementation := newProcessInboxBatchImplementation(repo)
	transactionManager := &countingTransactionManager{}
	implementation.tx = transactionManager

	err := implementation.ProcessInboxBatch(context.Background())
	if !errors.Is(err, processingErr) || !errors.Is(err, releaseErr) {
		t.Fatalf("ProcessInboxBatch() error = %v", err)
	}
	if repo.claimBotID != 11 || repo.listBotID != 11 || repo.listLimit != 2 {
		t.Fatalf("claim bot=%d list bot=%d limit=%d", repo.claimBotID, repo.listBotID, repo.listLimit)
	}
	if repo.renewCalls != len(items) || repo.deleteCalls != len(items) || repo.retryCalls != len(items) {
		t.Fatalf("renew=%d delete=%d retry=%d", repo.renewCalls, repo.deleteCalls, repo.retryCalls)
	}
	if repo.releaseCalls != 1 || repo.releasedLease != lease {
		t.Fatalf("release calls=%d lease=%+v", repo.releaseCalls, repo.releasedLease)
	}
	if transactionManager.calls != len(items)+1 {
		t.Fatalf("transactions=%d, want one list transaction plus one per item", transactionManager.calls)
	}
}

func TestProcessInboxBatchStopsAfterLeaseRenewalFailure(t *testing.T) {
	t.Parallel()

	renewErr := repository.ErrStaleLease
	repo := &processInboxBatchRepository{
		lease:    repository.Lease{BotID: 11, Owner: uuid.New(), Version: 4},
		claimed:  true,
		items:    []repository.InboxItem{{ID: 1}, {ID: 2}},
		renewErr: renewErr,
	}
	implementation := newProcessInboxBatchImplementation(repo)

	if err := implementation.ProcessInboxBatch(context.Background()); !errors.Is(err, renewErr) {
		t.Fatalf("ProcessInboxBatch() error = %v, want %v", err, renewErr)
	}
	if repo.renewCalls != 1 || repo.deleteCalls != 0 || repo.releaseCalls != 1 {
		t.Fatalf("renew=%d delete=%d release=%d", repo.renewCalls, repo.deleteCalls, repo.releaseCalls)
	}
}

func newProcessInboxBatchImplementation(repo repository.Inbox) *Implementation {
	return &Implementation{
		cfg: &config.Config{
			Telegram: config.Telegram{Bot: config.Bot{ID: 11}},
			Inbox: config.Inbox{
				BatchSize:     2,
				LeaseDuration: time.Minute,
				Retry: config.Retry{
					MaxAttempts:  3,
					InitialDelay: time.Second,
					MaxDelay:     time.Minute,
					Multiplier:   2,
				},
			},
		},
		inbox: repo,
		telegram: decodedUpdateTelegram{update: gateway.DecodedUpdate{
			Message: &gateway.DecodedMessage{SupportedContent: false},
		}},
		tx: passthroughTransactionManager{},
	}
}

type processInboxBatchRepository struct {
	repository.Inbox

	lease         repository.Lease
	claimed       bool
	items         []repository.InboxItem
	retryErr      error
	renewErr      error
	releaseErr    error
	claimBotID    int64
	listBotID     int64
	listLimit     int
	listCalls     int
	renewCalls    int
	deleteCalls   int
	retryCalls    int
	releaseCalls  int
	releasedLease repository.Lease
}

func (r *processInboxBatchRepository) ClaimInboxProcessor(
	_ context.Context,
	req repository.ClaimLeaseRequest,
) (*repository.Lease, bool, error) {
	r.claimBotID = req.BotID

	return &r.lease, r.claimed, nil
}

func (r *processInboxBatchRepository) ListInbox(
	_ context.Context,
	req repository.ListInboxRequest,
) ([]repository.InboxItem, error) {
	r.listCalls++
	r.listBotID = req.BotID
	r.listLimit = req.Limit

	return r.items, nil
}

func (r *processInboxBatchRepository) RenewInboxProcessor(
	context.Context,
	repository.Lease,
	time.Time,
) error {
	r.renewCalls++

	return r.renewErr
}

func (r *processInboxBatchRepository) DeleteInbox(context.Context, repository.Lease, int64) error {
	r.deleteCalls++

	return r.retryErr
}

func (r *processInboxBatchRepository) RetryInbox(
	context.Context,
	repository.RetryInboxRequest,
) error {
	r.retryCalls++

	return nil
}

func (r *processInboxBatchRepository) ReleaseInboxProcessor(_ context.Context, lease repository.Lease) error {
	r.releaseCalls++
	r.releasedLease = lease

	return r.releaseErr
}

type countingTransactionManager struct {
	calls int
}

func (m *countingTransactionManager) Do(ctx context.Context, fn func(context.Context) error) error {
	m.calls++

	return fn(ctx)
}

func (m *countingTransactionManager) DoWithSettings(
	ctx context.Context,
	_ trm.Settings,
	fn func(context.Context) error,
) error {
	m.calls++

	return fn(ctx)
}
