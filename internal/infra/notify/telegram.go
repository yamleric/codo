package notify

import (
	"context"
	"fmt"
	"os"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/codo/codo/internal/application/pipeline"
)

// Telegram implements pipeline.Notifier.
// userID is expected to be a Telegram chat ID stored as string.
type Telegram struct {
	bot *tgbotapi.BotAPI
}

func NewTelegram() (*Telegram, error) {
	token := os.Getenv("TELEGRAM_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("notify: TELEGRAM_TOKEN not set")
	}
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("notify: telegram init: %w", err)
	}
	return &Telegram{bot: bot}, nil
}

func (t *Telegram) Send(_ context.Context, userID, message string) error {
	chatID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return fmt.Errorf("notify: invalid telegram chat id %q: %w", userID, err)
	}
	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = tgbotapi.ModeMarkdown
	_, err = t.bot.Send(msg)
	return err
}

var _ pipeline.Notifier = (*Telegram)(nil)
