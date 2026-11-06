package chat //nolint:testpackage // The test inspects application-level dependency calls.

import (
	"context"
	"errors"
	"testing"
	"time"

	"tg-bot-proxy/config"
	"tg-bot-proxy/internal/domain/gateway"
	"tg-bot-proxy/internal/domain/repository"
)

func TestPollPreservesGatewayAndLeaseReleaseErrors(t *testing.T) {
	t.Parallel()

	gatewayErr := errors.New("get updates failed")
	releaseErr := errors.New("release ingestion failed")
	repo := &ingestionRepository{releaseErr: releaseErr}
	implementation := &Implementation{
		cfg: &config.Config{Telegram: config.Telegram{Bot: config.Bot{
			ID: 1,
			Polling: config.Polling{
				BatchSize:     10,
				Timeout:       time.Second,
				LeaseDuration: time.Minute,
			},
		}}},
		inbox:    repo,
		telegram: pollTelegram{err: gatewayErr},
	}

	err := implementation.Poll(context.Background())
	if !errors.Is(err, gatewayErr) || !errors.Is(err, releaseErr) {
		t.Fatalf("poll error=%v, want gateway and release errors", err)
	}
	if repo.releaseCalls != 1 {
		t.Fatalf("release calls=%d, want 1", repo.releaseCalls)
	}
}

func TestPollCreatesInboxItemsBeforeCommittingIngestion(t *testing.T) {
	t.Parallel()

	updates := []gateway.TelegramUpdate{
		{ID: 7, Payload: []byte(`{"update_id":7}`)},
		{ID: 9, Payload: []byte(`{"update_id":9}`)},
	}
	repo := &ingestionRepository{}
	implementation := &Implementation{
		cfg: &config.Config{Telegram: config.Telegram{Bot: config.Bot{
			ID: 1,
			Polling: config.Polling{
				BatchSize:     10,
				Timeout:       time.Second,
				LeaseDuration: time.Minute,
			},
		}}},
		inbox:    repo,
		telegram: pollTelegram{updates: updates},
		tx:       passthroughTransactionManager{},
	}

	if err := implementation.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.calls) != 2 || repo.calls[0] != "create" || repo.calls[1] != "commit" {
		t.Fatalf("repository calls=%v, want [create commit]", repo.calls)
	}
	if repo.createReq.BotID != 1 || len(repo.createReq.Updates) != len(updates) {
		t.Fatalf("create request=%+v", repo.createReq)
	}
	if repo.commitReq.Lease.BotID != 1 || repo.commitReq.NextOffset != 10 {
		t.Fatalf("commit request=%+v", repo.commitReq)
	}
}

type ingestionRepository struct {
	repository.Inbox

	releaseCalls int
	releaseErr   error
	calls        []string
	createReq    repository.CreateInboxItemsRequest
	commitReq    repository.CommitIngestionRequest
}

func (r *ingestionRepository) ClaimIngestion(
	context.Context,
	repository.ClaimLeaseRequest,
) (*repository.Lease, bool, error) {
	return &repository.Lease{BotID: 1}, true, nil
}

func (r *ingestionRepository) CreateInboxItems(
	_ context.Context,
	req repository.CreateInboxItemsRequest,
) error {
	r.calls = append(r.calls, "create")
	r.createReq = req

	return nil
}

func (r *ingestionRepository) CommitIngestion(
	_ context.Context,
	req repository.CommitIngestionRequest,
) error {
	r.calls = append(r.calls, "commit")
	r.commitReq = req

	return nil
}

func (r *ingestionRepository) ReleaseIngestion(context.Context, repository.Lease) error {
	r.releaseCalls++

	return r.releaseErr
}

type pollTelegram struct {
	gateway.Telegram

	updates []gateway.TelegramUpdate
	err     error
}

func (g pollTelegram) GetUpdates(context.Context, gateway.GetUpdatesRequest) ([]gateway.TelegramUpdate, error) {
	return g.updates, g.err
}
