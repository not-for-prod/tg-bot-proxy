package chat //nolint:testpackage // Shared fake supports focused worker-family tests.

import (
	"context"
	"os"
	"sync"
	"testing"

	"tg-bot-proxy/internal/domain/entity"
)

func TestMain(m *testing.M) {
	if err := os.Setenv("POSTGRES_DSN", "postgres://postgres@localhost:5432/tg_bot_proxy?sslmode=disable"); err != nil {
		panic(err)
	}
	if err := os.Setenv("TELEGRAM_BOT_TOKEN", "test-token"); err != nil {
		panic(err)
	}
	if err := os.Setenv("TELEGRAM_BOT_ID", "1"); err != nil {
		panic(err)
	}
	if err := os.Setenv("TELEGRAM_SUPERGROUP_ID", "-1001"); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

type fakeTelegramProxy struct {
	mutex                 sync.Mutex
	pollCalls             int
	inboxCalls            int
	operations            []entity.Operation
	claimLimits           []int
	claimOperationsCalls  int
	executeOperationCalls int
	activeOperations      int
	maxActiveOperations   int
	execute               func(context.Context, entity.Operation) error
}

func (f *fakeTelegramProxy) Poll(context.Context) error {
	f.pollCalls++

	return nil
}

func (f *fakeTelegramProxy) ProcessInboxBatch(context.Context) error {
	f.inboxCalls++

	return nil
}

func (f *fakeTelegramProxy) ClaimOperations(_ context.Context, capacity int) ([]entity.Operation, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.claimOperationsCalls++
	f.claimLimits = append(f.claimLimits, capacity)
	count := min(capacity, len(f.operations))
	operations := append([]entity.Operation(nil), f.operations[:count]...)
	f.operations = f.operations[count:]

	return operations, nil
}

func (f *fakeTelegramProxy) ExecuteOperation(ctx context.Context, operation entity.Operation) error {
	f.mutex.Lock()
	f.executeOperationCalls++
	f.activeOperations++
	f.maxActiveOperations = max(f.maxActiveOperations, f.activeOperations)
	f.mutex.Unlock()

	var err error
	if f.execute != nil {
		err = f.execute(ctx, operation)
	}

	f.mutex.Lock()
	f.activeOperations--
	f.mutex.Unlock()

	return err
}
