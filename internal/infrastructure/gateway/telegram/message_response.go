package telegram

import (
	"net/http"

	"github.com/go-resty/resty/v2"
)

type messageResult struct {
	MessageID *int64 `json:"message_id"`
}

func messageResponseError(
	response *resty.Response,
	body telegramResponse[messageResult],
	requestErr error,
) error {
	if requestErr != nil {
		if response != nil && response.RawResponse != nil {
			if response.StatusCode() >= http.StatusOK && response.StatusCode() < http.StatusMultipleChoices {
				return retryableMalformedTelegramResponse()
			}

			return malformedTelegramResponse()
		}

		return retryableTransportError()
	}
	if !body.OK {
		return telegramAPIError(response.StatusCode(), body.ErrorCode, body.Description, body.Parameters.RetryAfter)
	}
	if body.Result.MessageID == nil {
		return retryableMalformedTelegramResponse()
	}

	return nil
}
