package chat //nolint:testpackage // The test inspects the repository claim limit selected by the service.

import (
	"context"
	"testing"
	"time"

	"tg-bot-proxy/config"
	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/domain/repository"
)

func TestClaimOperationsCapsRequestedCapacityByConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		capacity    int
		batchSize   int
		poolSize    int
		expectLimit int
	}{
		{name: "free capacity", capacity: 2, batchSize: 20, poolSize: 3, expectLimit: 2},
		{name: "worker pool", capacity: 10, batchSize: 20, poolSize: 3, expectLimit: 3},
		{name: "claim batch", capacity: 10, batchSize: 2, poolSize: 3, expectLimit: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repo := &claimOperationsRepository{}
			implementation := &Implementation{
				cfg: &config.Config{Outbox: config.Outbox{
					BatchSize:      test.batchSize,
					WorkerPoolSize: test.poolSize,
					LeaseDuration:  time.Minute,
				}},
				outbox: repo,
			}

			if _, err := implementation.ClaimOperations(context.Background(), test.capacity); err != nil {
				t.Fatal(err)
			}
			if repo.limit != test.expectLimit {
				t.Fatalf("claim limit=%d, want %d", repo.limit, test.expectLimit)
			}
		})
	}
}

type claimOperationsRepository struct {
	repository.Outbox

	limit int
}

func (r *claimOperationsRepository) ClaimOperations(
	_ context.Context,
	req repository.ClaimOperationsRequest,
) ([]entity.Operation, error) {
	r.limit = req.Limit
	return nil, nil
}
