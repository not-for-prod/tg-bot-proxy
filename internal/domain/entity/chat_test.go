package entity_test

import (
	"testing"

	"tg-bot-proxy/internal/domain/entity"
)

func TestChatAllocatesOffsetsInOrder(t *testing.T) {
	t.Parallel()
	chat := entity.NewChat(10, -100)
	if offset := chat.AllocateOffset(); offset != 1 {
		t.Fatalf("first offset = %d, want 1", offset)
	}
	if offset := chat.AllocateOffset(); offset != 2 {
		t.Fatalf("second offset = %d, want 2", offset)
	}
}

func TestChatRejectsInvalidRehydration(t *testing.T) {
	t.Parallel()
	if _, err := entity.RehydrateChat(1, 2, -100, nil, 1, 1); err == nil {
		t.Fatal("expected invalid offset range to fail")
	}
}
