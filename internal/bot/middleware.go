package bot

import (
	"fmt"
	"runtime/debug"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// recoverMiddleware handles panics in message handlers
func (b *Bot) recoverMiddleware(handler func()) {
	defer func() {
		if r := recover(); r != nil {
			b.logger.Error().
				Interface("panic", r).
				Str("stack", string(debug.Stack())).
				Msg("Panic recovered in handler")
		}
	}()

	handler()
}

// sendErrorMessage sends an error message to the user
func (b *Bot) sendErrorMessage(chatID int64, errorMsg string) {
	msg := tgbotapi.NewMessage(chatID, errorMsg)
	_, err := b.api.Send(msg)
	if err != nil {
		b.logger.Error().
			Err(err).
			Int64("chat_id", chatID).
			Msg("Failed to send error message")
	}
}

// sendMessage sends a message to the chat
func (b *Bot) sendMessage(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	
	_, err := b.api.Send(msg)
	if err != nil {
		b.logger.Error().
			Err(err).
			Int64("chat_id", chatID).
			Msg("Failed to send message")
		return fmt.Errorf("failed to send message: %w", err)
	}
	
	return nil
}

// sendTypingAction sends typing action to the chat
func (b *Bot) sendTypingAction(chatID int64) {
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, _ = b.api.Send(action)
}
