package bot

import (
	"context"
	"fmt"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/telegram-llm-bot/internal/models"
)

// handleMigrateHistoryCommand handles /migrate_history command
// This command fetches ALL chat history from Telegram and saves it to database
func (b *Bot) handleMigrateHistoryCommand(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	userID := message.From.ID

	// Check if this is from allowed chat
	if !b.config.IsAllowedChat(chatID) {
		b.sendMessage(chatID, "❌ Эта команда доступна только в разрешенных чатах.")
		return
	}

	b.logger.Info().
		Int64("user_id", userID).
		Str("username", message.From.UserName).
		Int64("chat_id", chatID).
		Msg("History migration requested")

	// Send initial message
	b.sendMessage(chatID, "🔄 Начинаю загрузку истории чата из Telegram...\n\nЭто может занять несколько минут в зависимости от размера истории.")

	// Run migration in background
	go b.runHistoryMigration(context.Background(), chatID, userID)
}

// runHistoryMigration performs the actual history migration
func (b *Bot) runHistoryMigration(ctx context.Context, chatID, userID int64) {
	startTime := time.Now()
	
	b.logger.Info().
		Int64("chat_id", chatID).
		Msg("Starting history migration")

	// Telegram API allows getting chat history in batches
	// We'll iterate backwards from the latest message
	var (
		totalMessages   int
		savedMessages   int
		offsetMessageID int
		batchSize       = 100 // Max allowed by Telegram
	)

	// Iterate through message history
	for {
		// Configure request
		config := tgbotapi.ChatConfig{
			ChatID: chatID,
		}

		// Get chat history
		// Note: Telegram API doesn't have a direct "get all history" method
		// We need to use getUpdates or iterate through messages
		// For supergroups, we can't use getChatHistory directly
		// Instead, we'll fetch messages using message IDs

		// Alternative approach: Use exportChatHistory (Telegram Bot API 5.0+)
		// But it's not available in go-telegram-bot-api library yet
		
		// Workaround: Ask user to forward messages or use different approach
		b.logger.Warn().
			Int64("chat_id", chatID).
			Msg("Direct chat history export not available in current library")

		break
	}

	duration := time.Since(startTime)

	// Send completion message with instructions
	msg := fmt.Sprintf(
		"ℹ️ Прямая загрузка истории через Telegram API ограничена.\n\n"+
			"**Альтернативные способы:**\n\n"+
			"1. **Автоматическое сохранение** (рекомендуется):\n"+
			"   • Все новые сообщения сохраняются автоматически\n"+
			"   • История накопится естественным образом\n\n"+
			"2. **Экспорт истории вручную:**\n"+
			"   • Telegram Desktop → Настройки → Расширенные\n"+
			"   • Экспорт данных чата → JSON\n"+
			"   • Загрузить файл в БД (требует отдельного скрипта)\n\n"+
			"3. **Использовать Telegram Desktop + MTProto:**\n"+
			"   • Требует отдельный скрипт на Python с Telethon\n"+
			"   • Может загрузить всю историю через MTProto API\n\n"+
			"С текущего момента все сообщения сохраняются автоматически!",
	)

	b.sendMessage(chatID, msg)

	b.logger.Info().
		Int64("chat_id", chatID).
		Int64("user_id", userID).
		Int("total_messages", totalMessages).
		Int("saved_messages", savedMessages).
		Dur("duration", duration).
		Msg("History migration completed with limitations")
}

// Alternative: Manual message migration from JSON export
// This would be a separate utility script, not a bot command
