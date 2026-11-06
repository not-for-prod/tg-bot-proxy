package telegram_test

import (
	"errors"
	"fmt"
	"testing"

	telegramgateway "tg-bot-proxy/internal/infrastructure/gateway/telegram"

	"github.com/not-for-prod/proterror/proterror"
)

func TestDecodeUpdateAcceptsSupportedMedia(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{name: "photo", content: `"photo":[{}]`},
		{name: "video", content: `"video":{}`},
		{name: "animation", content: `"animation":{}`},
		{name: "audio", content: `"audio":{}`},
		{name: "voice", content: `"voice":{}`},
		{name: "document", content: `"document":{}`},
		{name: "sticker", content: `"sticker":{}`},
		{name: "location", content: `"location":{}`},
		{name: "contact", content: `"contact":{}`},
		{name: "caption", content: `"caption":"media caption"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			payload := fmt.Sprintf(
				`{"update_id":4,"message":{"message_id":9,"from":{"is_bot":false},"chat":{"id":1,"type":"private"},%s}}`,
				test.content,
			)
			update, err := (&telegramgateway.Implementation{}).DecodeUpdate([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			if update.Message == nil || !update.Message.SupportedContent {
				t.Fatalf("unexpected decoded message: %+v", update.Message)
			}
		})
	}
}

func TestDecodeUpdateAcceptsHumanTopicMessage(t *testing.T) {
	t.Parallel()

	implementation := &telegramgateway.Implementation{}
	update, err := implementation.DecodeUpdate([]byte(
		`{"update_id":4,"message":{"message_id":9,"message_thread_id":3,"from":{"is_bot":false,"first_name":"Ada"},"chat":{"id":-100,"type":"supergroup"},"text":"hello"}}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if update.Message == nil || !update.Message.SupportedContent || update.Message.FromBot ||
		update.Message.ThreadID != 3 || update.Message.FirstName != "Ada" {
		t.Fatalf("unexpected decoded message: %+v", update.Message)
	}
}

func TestDecodeUpdateClassifiesServiceContent(t *testing.T) {
	t.Parallel()

	implementation := &telegramgateway.Implementation{}
	update, err := implementation.DecodeUpdate([]byte(
		`{"update_id":4,"message":{"message_id":9,"from":{"is_bot":false},"chat":{"id":-100,"type":"supergroup"},"forum_topic_created":{"name":"topic"}}}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if update.Message == nil || update.Message.SupportedContent {
		t.Fatalf("unexpected decoded message: %+v", update.Message)
	}
}

func TestDecodeUpdateRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	implementation := &telegramgateway.Implementation{}
	if _, err := implementation.DecodeUpdate([]byte(`{"message":`)); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestDecodeUpdateRejectsInvalidMessageIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "missing message ID",
			payload: `{"message":{"from":{"is_bot":false},"chat":{"id":1,"type":"private"},"text":"hello"}}`,
		},
		{
			name:    "zero message ID",
			payload: `{"message":{"message_id":0,"from":{"is_bot":false},"chat":{"id":1,"type":"private"},"text":"hello"}}`,
		},
		{
			name:    "missing chat ID",
			payload: `{"message":{"message_id":1,"from":{"is_bot":false},"chat":{"type":"private"},"text":"hello"}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := (&telegramgateway.Implementation{}).DecodeUpdate([]byte(test.payload))
			if !errors.Is(err, &proterror.Internal{}) {
				t.Fatalf("expected malformed provider response, got %v", err)
			}
		})
	}
}
