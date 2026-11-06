package telegram

import (
	"encoding/json"
	"errors"

	"tg-bot-proxy/internal/domain/gateway"

	"github.com/not-for-prod/proterror/proterror"
)

type telegramUpdate struct {
	Message *telegramMessage `json:"message"`
}

type telegramMessage struct {
	ID        int64           `json:"message_id"`
	ThreadID  int64           `json:"message_thread_id"`
	From      *telegramUser   `json:"from"`
	Chat      telegramChat    `json:"chat"`
	Text      string          `json:"text"`
	Caption   string          `json:"caption"`
	Photo     json.RawMessage `json:"photo"`
	Video     json.RawMessage `json:"video"`
	Animation json.RawMessage `json:"animation"`
	Audio     json.RawMessage `json:"audio"`
	Voice     json.RawMessage `json:"voice"`
	Document  json.RawMessage `json:"document"`
	Sticker   json.RawMessage `json:"sticker"`
	Location  json.RawMessage `json:"location"`
	Contact   json.RawMessage `json:"contact"`
}

type telegramUser struct {
	IsBot     bool   `json:"is_bot"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type telegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

func (i *Implementation) DecodeUpdate(payload []byte) (gateway.DecodedUpdate, error) {
	var update telegramUpdate
	if err := json.Unmarshal(payload, &update); err != nil {
		return gateway.DecodedUpdate{}, err
	}
	if update.Message == nil {
		return gateway.DecodedUpdate{}, nil
	}

	message := update.Message
	if message.ID <= 0 || message.Chat.ID == 0 {
		return gateway.DecodedUpdate{}, errors.Join(
			&proterror.Internal{},
			errors.New("telegram message has an invalid identity"),
		)
	}
	supportedContent := message.Text != "" || message.Caption != "" || len(message.Photo) > 0 ||
		len(message.Video) > 0 || len(message.Animation) > 0 || len(message.Audio) > 0 ||
		len(message.Voice) > 0 || len(message.Document) > 0 || len(message.Sticker) > 0 ||
		len(message.Location) > 0 || len(message.Contact) > 0
	result := &gateway.DecodedMessage{
		ID:               message.ID,
		ThreadID:         message.ThreadID,
		ChatID:           message.Chat.ID,
		ChatType:         message.Chat.Type,
		Text:             message.Text,
		SupportedContent: supportedContent,
	}
	if result.Text == "" {
		result.Text = message.Caption
	}
	if message.From == nil {
		result.FromBot = true

		return gateway.DecodedUpdate{Message: result}, nil
	}

	result.FromBot = message.From.IsBot
	result.Username = message.From.Username
	result.FirstName = message.From.FirstName
	result.LastName = message.From.LastName

	return gateway.DecodedUpdate{Message: result}, nil
}
