package outboxrepository //nolint:testpackage // Payload decoding is an adapter-local persistence contract.

import (
	"reflect"
	"testing"

	"tg-bot-proxy/internal/domain/entity"
)

func TestUnmarshalOperationPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		operationType entity.OperationType
		encoded       string
		want          entity.OperationPayload
	}{
		{
			name:          "external relay",
			operationType: entity.OperationRelayExternalToInternal,
			encoded:       `{"sourceChatId":101,"sourceMessageId":202,"topicTitle":"Support"}`,
			want: entity.RelayExternalToInternalPayload{
				SourceChatID: 101, SourceMessageID: 202, TopicTitle: "Support",
			},
		},
		{
			name:          "internal relay",
			operationType: entity.OperationRelayInternalToExternal,
			encoded:       `{"sourceChatId":-100,"sourceMessageId":303}`,
			want:          entity.RelayInternalToExternalPayload{SourceChatID: -100, SourceMessageID: 303},
		},
		{
			name:          "send message",
			operationType: entity.OperationSendExternalMessage,
			encoded:       `{"text":"Welcome"}`,
			want:          entity.SendExternalMessagePayload{Text: "Welcome"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			payload, err := unmarshalOperationPayload(test.operationType, []byte(test.encoded))
			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(payload, test.want) {
				t.Fatalf("decoded=%+v, want %+v", payload, test.want)
			}
		})
	}
}

func TestUnmarshalOperationPayloadReturnsInvalidMarker(t *testing.T) {
	t.Parallel()

	payload, err := unmarshalOperationPayload(entity.OperationSendExternalMessage, []byte(`{"text":5}`))
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := payload.(entity.InvalidOperationPayload); !ok {
		t.Fatalf("malformed payload=%T, want invalid marker", payload)
	}
}
