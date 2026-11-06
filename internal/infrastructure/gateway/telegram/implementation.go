package telegram

import (
	"errors"
	"strings"

	"tg-bot-proxy/config"
	"tg-bot-proxy/internal/domain/gateway"

	"github.com/go-resty/resty/v2"
	"github.com/not-for-prod/proterror/proterror"
)

var _ gateway.Telegram = &Implementation{}

type Implementation struct {
	cfg    *config.Telegram
	client *resty.Client
}

type responseParameters struct {
	RetryAfter int `json:"retry_after"`
}

type telegramResponse[T any] struct {
	OK          bool               `json:"ok"`
	Result      T                  `json:"result"`
	ErrorCode   int                `json:"error_code"`
	Description string             `json:"description"`
	Parameters  responseParameters `json:"parameters"`
}

func NewImplementation() *Implementation {
	cfg := &config.Instance().Telegram
	client := resty.New().
		SetBaseURL(cfg.BaseURL).
		SetTimeout(cfg.RequestTimeout).
		SetHeader("Content-Type", "application/json")

	return &Implementation{cfg: cfg, client: client}
}

func methodPath(token, method string) string {
	return "/bot" + token + "/" + method
}

func telegramAPIError(httpStatus, code int, description string, retryAfter int) error {
	if code == 0 {
		code = httpStatus
	}

	lower := strings.ToLower(description)
	kind := gateway.TelegramOutcomePermanent
	if strings.Contains(lower, "message thread not found") ||
		strings.Contains(lower, "topic") && strings.Contains(lower, "not found") ||
		(strings.Contains(lower, "reply") || strings.Contains(lower, "replied")) &&
			strings.Contains(lower, "not found") {
		kind = gateway.TelegramOutcomeMissingTopic
	} else if httpStatus == 429 || httpStatus >= 500 || code == 429 || code >= 500 {
		kind = gateway.TelegramOutcomeRetryable
	}

	return &gateway.TelegramError{
		Code:        code,
		HTTPStatus:  httpStatus,
		Description: safeTelegramErrorDescription(kind),
		RetryAfter:  retryAfter,
		Kind:        kind,
	}
}

func safeTelegramErrorDescription(kind gateway.TelegramOutcomeKind) string {
	switch kind {
	case gateway.TelegramOutcomeMissingTopic:
		return "telegram topic not found"
	case gateway.TelegramOutcomeRetryable:
		return "telegram request temporarily unavailable"
	case gateway.TelegramOutcomeAmbiguous:
		return "telegram request outcome is unknown"
	case gateway.TelegramOutcomePermanent:
		return "telegram request rejected"
	default:
		return "telegram request failed"
	}
}

func retryableTransportError() error {
	return errors.Join(
		&proterror.Unavailable{},
		&gateway.TelegramError{
			Description: "telegram transport unavailable",
			Kind:        gateway.TelegramOutcomeRetryable,
		},
	)
}

func malformedTelegramResponse() error {
	return errors.Join(
		&proterror.Internal{},
		errors.New("telegram returned a malformed response"),
	)
}

func retryableMalformedTelegramResponse() error {
	return errors.Join(
		malformedTelegramResponse(),
		&gateway.TelegramError{
			Description: "telegram message response is temporarily unusable",
			Kind:        gateway.TelegramOutcomeRetryable,
		},
	)
}
