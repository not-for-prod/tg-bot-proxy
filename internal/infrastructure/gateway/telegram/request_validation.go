package telegram

import (
	"strings"
	"unicode/utf8"
)

const (
	telegramMessageMaxRunes = 4096
	telegramTopicMaxRunes   = 128
)

func validTelegramText(value string, maxRunes int) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && utf8.RuneCountInString(value) <= maxRunes
}
