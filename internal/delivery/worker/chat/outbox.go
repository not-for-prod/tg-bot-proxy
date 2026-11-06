package chat

import (
	"context"
	"errors"
	"sync"

	"tg-bot-proxy/config"
	"tg-bot-proxy/internal/domain/entity"
	"tg-bot-proxy/internal/domain/service"
	sharedworker "tg-bot-proxy/internal/pkg/worker"
)

func NewOutboxPublisher(
	proxy service.TelegramProxy,
	metrics *sharedworker.Metrics,
) sharedworker.Worker {
	cfg := config.Instance().Outbox

	return sharedworker.New(
		sharedworker.Config{
			Name:     "telegram-outbox-publisher",
			Interval: cfg.Interval,
		},
		func(ctx context.Context) error {
			return runOutboxPublisher(ctx, cfg.WorkerPoolSize, proxy)
		},
		metrics,
	)
}

func runOutboxPublisher(ctx context.Context, workerCount int, proxy service.TelegramProxy) error {
	jobs := make(chan entity.Operation)
	results := make(chan error, workerCount)
	var workers sync.WaitGroup
	for range workerCount {
		workers.Go(func() {
			for operation := range jobs {
				results <- proxy.ExecuteOperation(ctx, operation)
			}
		})
	}

	err := dispatchReadyOperations(ctx, dispatchReadyOperationsRequest{
		WorkerCount: workerCount,
		Proxy:       proxy,
		Jobs:        jobs,
		Results:     results,
	})
	close(jobs)
	workers.Wait()

	return err
}

type dispatchReadyOperationsRequest struct {
	WorkerCount int
	Proxy       service.TelegramProxy
	Jobs        chan<- entity.Operation
	Results     <-chan error
}

func dispatchReadyOperations(ctx context.Context, req dispatchReadyOperationsRequest) error {
	inFlight := 0
	claiming := true
	var operationErrors []error
	for {
		for claiming && inFlight < req.WorkerCount {
			operations, err := req.Proxy.ClaimOperations(ctx, req.WorkerCount-inFlight)
			if err != nil {
				operationErrors = append(operationErrors, err)
				claiming = false

				break
			}
			if len(operations) == 0 {
				break
			}

			for _, operation := range operations {
				req.Jobs <- operation
				inFlight++
			}
		}

		if inFlight == 0 {
			return errors.Join(operationErrors...)
		}

		if err := <-req.Results; err != nil {
			operationErrors = append(operationErrors, err)
		}
		inFlight--
	}
}
