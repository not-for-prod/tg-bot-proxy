package outboxrepository //nolint:testpackage // Payload encoding is an adapter-local persistence contract.

import (
	"errors"
	"testing"

	"tg-bot-proxy/internal/domain/entity"

	"github.com/not-for-prod/proterror/proterror"
)

func TestMarshalOperationPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		operationType entity.OperationType
		payload       entity.OperationPayload
		wantJSON      string
	}{
		{
			name: "external relay", operationType: entity.OperationRelayExternalToInternal,
			payload: entity.RelayExternalToInternalPayload{
				SourceChatID: 101, SourceMessageID: 202, TopicTitle: "Support",
			},
			wantJSON: `{"sourceChatId":101,"sourceMessageId":202,"topicTitle":"Support"}`,
		},
		{
			name: "internal relay", operationType: entity.OperationRelayInternalToExternal,
			payload:  entity.RelayInternalToExternalPayload{SourceChatID: -100, SourceMessageID: 303},
			wantJSON: `{"sourceChatId":-100,"sourceMessageId":303}`,
		},
		{
			name: "send message", operationType: entity.OperationSendExternalMessage,
			payload:  entity.SendExternalMessagePayload{Text: "Welcome"},
			wantJSON: `{"text":"Welcome"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := marshalOperationPayload(test.operationType, test.payload)
			if err != nil {
				t.Fatal(err)
			}

			if string(encoded) != test.wantJSON {
				t.Fatalf("encoded=%s, want %s", encoded, test.wantJSON)
			}
		})
	}
}

func TestMarshalOperationPayloadRejectsInvalidContract(t *testing.T) {
	t.Parallel()

	_, err := marshalOperationPayload(
		entity.OperationSendExternalMessage,
		entity.RelayInternalToExternalPayload{},
	)
	if !errors.Is(err, &proterror.Internal{}) {
		t.Fatalf("mismatched payload error=%v", err)
	}
}
