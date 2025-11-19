package bot

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

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

// escapeMarkdown escapes special Markdown characters for MarkdownV2
func escapeMarkdown(text string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(text)
}

// sendMessage sends a message to the chat with multiple fallback strategies
func (b *Bot) sendMessage(chatID int64, text string) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return b.sendMessageWithContext(ctx, chatID, text)
}

// sendMessageWithContext sends a message with a specific context
func (b *Bot) sendMessageWithContext(ctx context.Context, chatID int64, text string) error {
	// Check message length and truncate if needed
	if len(text) > 4096 {
		b.logger.Warn().
			Int64("chat_id", chatID).
			Int("text_length", len(text)).
			Msg("Message too long for Telegram, truncating")
		text = text[:4090] + "..."
	}

	// Channel for result
	type result struct {
		err error
	}
	resultChan := make(chan result, 1)

	go func() {
		// Attempt 1: Try with Markdown
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"

		_, err := b.api.Send(msg)
		if err != nil {
			b.logger.Warn().
				Err(err).
				Int64("chat_id", chatID).
				Msg("Failed to send message with Markdown, trying with escaped MarkdownV2")

			// Attempt 2: Try with escaped MarkdownV2
			msgEscaped := tgbotapi.NewMessage(chatID, escapeMarkdown(text))
			msgEscaped.ParseMode = "MarkdownV2"

			_, err2 := b.api.Send(msgEscaped)
			if err2 != nil {
				b.logger.Warn().
					Err(err2).
					Int64("chat_id", chatID).
					Msg("Failed with escaped MarkdownV2, sending as plain text")

				// Attempt 3: Send without any formatting
				msgPlain := tgbotapi.NewMessage(chatID, text)
				msgPlain.ParseMode = ""

				_, err3 := b.api.Send(msgPlain)
				if err3 != nil {
					b.logger.Error().
						Err(err3).
						Int64("chat_id", chatID).
						Msg("Failed to send message even as plain text")
					resultChan <- result{err: fmt.Errorf("failed to send message after 3 attempts: %w", err3)}
					return
				}
			}

			b.logger.Info().
				Int64("chat_id", chatID).
				Msg("Message sent successfully after retry")
		}

		resultChan <- result{err: nil}
	}()

	// Wait for result or timeout
	select {
	case <-ctx.Done():
		b.logger.Error().
			Int64("chat_id", chatID).
			Msg("Send message timeout exceeded")
		return fmt.Errorf("send message timeout: %w", ctx.Err())
	case res := <-resultChan:
		return res.err
	}
}

// sendTypingAction sends typing action to the chat
func (b *Bot) sendTypingAction(chatID int64) {
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, _ = b.api.Send(action)
}
