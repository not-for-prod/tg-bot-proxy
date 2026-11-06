package telegram //nolint:testpackage // classification helpers are adapter-local.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"tg-bot-proxy/config"
	"tg-bot-proxy/internal/domain/gateway"
	"tg-bot-proxy/internal/domain/repository"

	"github.com/go-resty/resty/v2"
	"github.com/not-for-prod/proterror/proterror"
)

func TestTelegramAPIErrorClassification(t *testing.T) {
	t.Parallel()
	err := telegramAPIError(http.StatusTooManyRequests, 429, "Too Many Requests", 12)
	var telegramErr *gateway.TelegramError
	if !errors.As(err, &telegramErr) || telegramErr.Kind != gateway.TelegramOutcomeRetryable ||
		telegramErr.RetryAfter != 12 {
		t.Fatalf("unexpected classification: %v", err)
	}
	if telegramErr.HTTPStatus != http.StatusTooManyRequests {
		t.Fatalf("unexpected HTTP status: %d", telegramErr.HTTPStatus)
	}

	err = telegramAPIError(http.StatusBadRequest, 400, "Bad Request: message thread not found", 0)
	if !errors.As(err, &telegramErr) || telegramErr.Kind != gateway.TelegramOutcomeMissingTopic {
		t.Fatalf("unexpected missing-topic classification: %v", err)
	}

	err = telegramAPIError(http.StatusBadRequest, 400, "Bad Request: message to be replied not found", 0)
	if !errors.As(err, &telegramErr) || telegramErr.Kind != gateway.TelegramOutcomeMissingTopic {
		t.Fatalf("unexpected missing-topic reply classification: %v", err)
	}
}

func TestCopyMessagePinsTopicRootReply(t *testing.T) {
	t.Parallel()

	topicID := int64(44)
	request := validCopyMessageRequest()
	request.TopicID = &topicID

	var transportRequest copyMessageRequest
	implementation := newTestImplementation(t, roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(request.Body).Decode(&transportRequest); err != nil {
			t.Fatal(err)
		}

		return telegramHTTPResponse(http.StatusOK, `{"ok":true,"result":{"message_id":55}}`), nil
	}))

	if err := implementation.CopyMessage(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if transportRequest.MessageThreadID == nil || *transportRequest.MessageThreadID != topicID {
		t.Fatalf("message thread ID=%v, want %d", transportRequest.MessageThreadID, topicID)
	}
	if transportRequest.ReplyParameters == nil || transportRequest.ReplyParameters.MessageID != topicID {
		t.Fatalf("reply parameters=%+v, want topic root %d", transportRequest.ReplyParameters, topicID)
	}
	if transportRequest.ReplyParameters.AllowSendingWithoutReply {
		t.Fatal("topic copy must fail when its root message no longer exists")
	}
}

func TestCopyMessageWithoutTopicOmitsReplyParameters(t *testing.T) {
	t.Parallel()

	var transportRequest copyMessageRequest
	implementation := newTestImplementation(t, roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(request.Body).Decode(&transportRequest); err != nil {
			t.Fatal(err)
		}

		return telegramHTTPResponse(http.StatusOK, `{"ok":true,"result":{"message_id":55}}`), nil
	}))

	if err := implementation.CopyMessage(context.Background(), validCopyMessageRequest()); err != nil {
		t.Fatal(err)
	}
	if transportRequest.MessageThreadID != nil || transportRequest.ReplyParameters != nil {
		t.Fatalf("unexpected topic targeting: %+v", transportRequest)
	}
}

func TestCopyMessageDecodesTelegramErrorResponse(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantCode   int
		wantKind   gateway.TelegramOutcomeKind
		wantAfter  int
	}{
		{
			name:       "bad request",
			statusCode: http.StatusBadRequest,
			body:       `{"ok":false,"error_code":400,"description":"Bad Request"}`,
			wantCode:   400,
			wantKind:   gateway.TelegramOutcomePermanent,
		},
		{
			name:       "rate limit",
			statusCode: http.StatusTooManyRequests,
			body:       `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":12}}`,
			wantCode:   429,
			wantKind:   gateway.TelegramOutcomeRetryable,
			wantAfter:  12,
		},
		{
			name:       "server error",
			statusCode: http.StatusServiceUnavailable,
			body:       `{"ok":false,"error_code":500,"description":"Internal Server Error"}`,
			wantCode:   500,
			wantKind:   gateway.TelegramOutcomeRetryable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			implementation := newTestImplementation(t, roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return telegramHTTPResponse(test.statusCode, test.body), nil
			}))

			err := implementation.CopyMessage(context.Background(), validCopyMessageRequest())
			var telegramErr *gateway.TelegramError
			if !errors.As(err, &telegramErr) {
				t.Fatalf("expected TelegramError, got %v", err)
			}
			if telegramErr.Code != test.wantCode || telegramErr.HTTPStatus != test.statusCode ||
				telegramErr.Kind != test.wantKind ||
				telegramErr.RetryAfter != test.wantAfter {
				t.Fatalf("unexpected TelegramError: %+v", telegramErr)
			}
		})
	}
}

func TestTelegramResponseDescriptionDoesNotEscapeGateway(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		code       int
		wantKind   gateway.TelegramOutcomeKind
		call       func(context.Context, *Implementation) error
	}{
		{
			name:       "copy message permanent error",
			statusCode: http.StatusBadRequest,
			code:       400,
			wantKind:   gateway.TelegramOutcomeMissingTopic,
			call: func(ctx context.Context, implementation *Implementation) error {
				return implementation.CopyMessage(ctx, validCopyMessageRequest())
			},
		},
		{
			name:       "create topic ambiguous error",
			statusCode: http.StatusServiceUnavailable,
			code:       500,
			wantKind:   gateway.TelegramOutcomeAmbiguous,
			call: func(ctx context.Context, implementation *Implementation) error {
				_, err := implementation.CreateForumTopic(ctx, "topic")
				return err
			},
		},
	}

	providerDescription := "message thread not found; provider echoed " + testBotToken
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := fmt.Sprintf(
				`{"ok":false,"error_code":%d,"description":%q}`,
				test.code,
				providerDescription,
			)
			implementation := newTestImplementation(t, roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return telegramHTTPResponse(test.statusCode, body), nil
			}))

			err := test.call(context.Background(), implementation)
			var telegramErr *gateway.TelegramError
			if !errors.As(err, &telegramErr) {
				t.Fatalf("expected TelegramError, got %v", err)
			}
			if telegramErr.Kind != test.wantKind {
				t.Fatalf("unexpected TelegramError kind: %s", telegramErr.Kind)
			}

			failure := repository.Failure{Message: err.Error()}
			if strings.Contains(err.Error(), testBotToken) || strings.Contains(telegramErr.Description, testBotToken) {
				t.Fatalf("outward error contains token: %v", err)
			}
			if strings.Contains(failure.Message, testBotToken) {
				t.Fatalf("persisted failure contains token: %s", failure.Message)
			}
		})
	}
}

func TestCopyMessageSanitizesRetryableTransportError(t *testing.T) {
	implementation := newTestImplementation(t, roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: request.Method, URL: request.URL.String(), Err: errors.New("connection reset")}
	}))

	err := implementation.CopyMessage(context.Background(), validCopyMessageRequest())
	var telegramErr *gateway.TelegramError
	if !errors.As(err, &telegramErr) || telegramErr.Kind != gateway.TelegramOutcomeRetryable {
		t.Fatalf("expected retryable TelegramError, got %v", err)
	}
	if !errors.Is(err, &proterror.Unavailable{}) {
		t.Fatalf("expected unavailable classification, got %v", err)
	}
	if strings.Contains(err.Error(), testBotToken) || strings.Contains(err.Error(), "/bot") {
		t.Fatalf("transport error contains token-bearing URL: %v", err)
	}
}

//nolint:gocognit // The matrix verifies the shared response policy across both message methods.
func TestMessageMethodsValidateResultMessageID(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "zero message ID is present", body: `{"ok":true,"result":{"message_id":0}}`},
		{name: "positive message ID", body: `{"ok":true,"result":{"message_id":42}}`},
		{name: "missing message ID", body: `{"ok":true,"result":{}}`, wantErr: true},
		{name: "null message ID", body: `{"ok":true,"result":{"message_id":null}}`, wantErr: true},
		{name: "wrong message ID type", body: `{"ok":true,"result":{"message_id":"42"}}`, wantErr: true},
		{name: "wrong result shape", body: `{"ok":true,"result":[]}`, wantErr: true},
	}

	methods := []struct {
		name string
		call func(context.Context, *Implementation) error
	}{
		{
			name: "copyMessage",
			call: func(ctx context.Context, implementation *Implementation) error {
				return implementation.CopyMessage(ctx, validCopyMessageRequest())
			},
		},
		{
			name: "sendMessage",
			call: func(ctx context.Context, implementation *Implementation) error {
				return implementation.SendMessage(ctx, 1, "hello")
			},
		},
	}

	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
						return telegramHTTPResponse(http.StatusOK, test.body), nil
					})
					implementation := newTestImplementation(t, transport)

					err := method.call(context.Background(), implementation)
					if !test.wantErr {
						if err != nil {
							t.Fatal(err)
						}

						return
					}
					if !errors.Is(err, &proterror.Internal{}) {
						t.Fatalf("expected malformed provider response, got %v", err)
					}
					var telegramErr *gateway.TelegramError
					if !errors.As(err, &telegramErr) || telegramErr.Kind != gateway.TelegramOutcomeRetryable {
						t.Fatalf("expected retryable malformed response, got %v", err)
					}
				})
			}
		})
	}
}

func TestGetUpdatesRequiresResultArray(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "empty result", body: `{"ok":true,"result":[]}`},
		{name: "missing result", body: `{"ok":true}`, wantErr: true},
		{name: "null result", body: `{"ok":true,"result":null}`, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			implementation := newTestImplementation(t, roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return telegramHTTPResponse(http.StatusOK, test.body), nil
			}))

			updates, err := implementation.GetUpdates(context.Background(), gateway.GetUpdatesRequest{})
			if !test.wantErr {
				if err != nil {
					t.Fatal(err)
				}
				if len(updates) != 0 {
					t.Fatalf("updates=%v, want none", updates)
				}

				return
			}
			if !errors.Is(err, &proterror.Internal{}) {
				t.Fatalf("expected malformed provider response, got %v", err)
			}
		})
	}
}

func TestGetUpdatesRequiresUpdateID(t *testing.T) {
	tests := []struct {
		name    string
		update  string
		wantID  int64
		wantErr bool
	}{
		{name: "zero", update: `{"update_id":0}`, wantID: 0},
		{name: "positive", update: `{"update_id":42}`, wantID: 42},
		{name: "missing", update: `{}`, wantErr: true},
		{name: "null", update: `{"update_id":null}`, wantErr: true},
		{name: "negative", update: `{"update_id":-1}`, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := `{"ok":true,"result":[` + test.update + `]}`
			implementation := newTestImplementation(t, roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return telegramHTTPResponse(http.StatusOK, body), nil
			}))

			updates, err := implementation.GetUpdates(context.Background(), gateway.GetUpdatesRequest{})
			if test.wantErr {
				if !errors.Is(err, &proterror.Internal{}) {
					t.Fatalf("expected malformed update, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(updates) != 1 || updates[0].ID != test.wantID {
				t.Fatalf("updates=%+v, want update_id %d", updates, test.wantID)
			}
		})
	}
}

func TestCreateForumTopicClassifiesTransportErrorByDispatchPhase(t *testing.T) {
	tests := []struct {
		name     string
		cause    error
		wantKind gateway.TelegramOutcomeKind
	}{
		{
			name:     "dial failure is retryable",
			cause:    &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
			wantKind: gateway.TelegramOutcomeRetryable,
		},
		{
			name:     "connection reset is ambiguous",
			cause:    errors.New("connection reset"),
			wantKind: gateway.TelegramOutcomeAmbiguous,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
				return nil, &url.Error{Op: request.Method, URL: request.URL.String(), Err: test.cause}
			})
			implementation := newTestImplementation(t, transport)

			_, err := implementation.CreateForumTopic(context.Background(), "topic")
			var telegramErr *gateway.TelegramError
			if !errors.As(err, &telegramErr) || telegramErr.Kind != test.wantKind {
				t.Fatalf("TelegramError=%v, want kind %s", err, test.wantKind)
			}
			if strings.Contains(err.Error(), testBotToken) || strings.Contains(err.Error(), "/bot") {
				t.Fatalf("transport error contains token-bearing URL: %v", err)
			}
		})
	}
}

func TestCreateForumTopicClassifiesResponseErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantCode   int
		wantKind   gateway.TelegramOutcomeKind
		wantAfter  int
	}{
		{
			name:       "server error is ambiguous",
			statusCode: http.StatusServiceUnavailable,
			body:       `{"ok":false,"error_code":500,"description":"Internal Server Error"}`,
			wantCode:   500,
			wantKind:   gateway.TelegramOutcomeAmbiguous,
		},
		{
			name:       "rate limit is retryable",
			statusCode: http.StatusTooManyRequests,
			body:       `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":12}}`,
			wantCode:   429,
			wantKind:   gateway.TelegramOutcomeRetryable,
			wantAfter:  12,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			implementation := newTestImplementation(t, roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return telegramHTTPResponse(test.statusCode, test.body), nil
			}))

			_, err := implementation.CreateForumTopic(context.Background(), "topic")
			var telegramErr *gateway.TelegramError
			if !errors.As(err, &telegramErr) {
				t.Fatalf("expected TelegramError, got %v", err)
			}
			if telegramErr.Code != test.wantCode || telegramErr.HTTPStatus != test.statusCode ||
				telegramErr.Kind != test.wantKind ||
				telegramErr.RetryAfter != test.wantAfter {
				t.Fatalf("unexpected TelegramError: %+v", telegramErr)
			}
		})
	}
}

func TestMessageMethodsRejectInvalidRequestsBeforeTransport(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context, *Implementation) error
	}{
		{
			name: "copy missing source chat",
			call: func(ctx context.Context, implementation *Implementation) error {
				request := validCopyMessageRequest()
				request.SourceChatID = 0
				return implementation.CopyMessage(ctx, request)
			},
		},
		{
			name: "copy invalid source message",
			call: func(ctx context.Context, implementation *Implementation) error {
				request := validCopyMessageRequest()
				request.SourceMessageID = 0
				return implementation.CopyMessage(ctx, request)
			},
		},
		{
			name: "copy invalid topic",
			call: func(ctx context.Context, implementation *Implementation) error {
				request := validCopyMessageRequest()
				invalidTopicID := int64(0)
				request.TopicID = &invalidTopicID
				return implementation.CopyMessage(ctx, request)
			},
		},
		{
			name: "send blank text",
			call: func(ctx context.Context, implementation *Implementation) error {
				return implementation.SendMessage(ctx, 1, " \n\t")
			},
		},
		{
			name: "send oversized text",
			call: func(ctx context.Context, implementation *Implementation) error {
				return implementation.SendMessage(ctx, 1, strings.Repeat("界", 4097))
			},
		},
		{
			name: "topic blank title",
			call: func(ctx context.Context, implementation *Implementation) error {
				_, err := implementation.CreateForumTopic(ctx, "  ")
				return err
			},
		},
		{
			name: "topic missing destination",
			call: func(ctx context.Context, implementation *Implementation) error {
				implementation.cfg.SupergroupID = 0
				_, err := implementation.CreateForumTopic(ctx, "Support")
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transportCalls := 0
			implementation := newTestImplementation(t, roundTripperFunc(func(*http.Request) (*http.Response, error) {
				transportCalls++
				return telegramHTTPResponse(http.StatusOK, `{"ok":true,"result":{"message_id":1}}`), nil
			}))

			err := test.call(context.Background(), implementation)
			if !errors.Is(err, &proterror.InvalidArgument{}) {
				t.Fatalf("error=%v, want invalid argument", err)
			}
			if transportCalls != 0 {
				t.Fatalf("transport calls=%d, want 0", transportCalls)
			}
		})
	}
}

const testBotToken = "test-token"

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newTestImplementation(t *testing.T, transport http.RoundTripper) *Implementation {
	t.Helper()

	cfg := &config.Telegram{
		BaseURL:        "https://api.telegram.org",
		RequestTimeout: time.Second,
		Bot:            config.Bot{Token: testBotToken},
		SupergroupID:   -1001,
	}
	client := resty.New().
		SetBaseURL(cfg.BaseURL).
		SetTimeout(cfg.RequestTimeout).
		SetHeader("Content-Type", "application/json").
		SetTransport(transport)

	return &Implementation{cfg: cfg, client: client}
}

func validCopyMessageRequest() gateway.CopyMessageRequest {
	return gateway.CopyMessageRequest{
		SourceChatID:      101,
		SourceMessageID:   202,
		DestinationChatID: -1001,
	}
}

func telegramHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
