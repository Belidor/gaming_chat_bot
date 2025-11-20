package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/telegram-llm-bot/internal/models"
)

const (
	// MaxQuestionLength is the maximum allowed length for a user question in characters
	MaxQuestionLength = 2000
)

// handleUpdate processes incoming update
func (b *Bot) handleUpdate(ctx context.Context, update tgbotapi.Update) {
	// Wrap in recover middleware
	b.recoverMiddleware(func() {
		// Handle message
		if update.Message != nil {
			b.handleMessage(ctx, update.Message)
		}
	})
}

// handleMessage processes incoming message
func (b *Bot) handleMessage(ctx context.Context, message *tgbotapi.Message) {
	// Handle commands from any chat (including private messages)
	if message.IsCommand() {
		b.handleCommand(ctx, message)
		return
	}

	// Only process non-command messages from allowed chats
	if !b.config.IsAllowedChat(message.Chat.ID) {
		b.logger.Debug().
			Int64("chat_id", message.Chat.ID).
			Msg("Ignoring message from non-allowed chat")
		return
	}

	// Save ALL messages to chat_messages for RAG (async, non-blocking)
	go b.saveChatMessage(ctx, message)

	// Check if message contains bot mention
	if b.isMentioned(message) {
		b.handleMention(ctx, message)
		return
	}
}

// handleCommand processes bot commands
func (b *Bot) handleCommand(ctx context.Context, message *tgbotapi.Message) {
	command := message.Command()

	b.logger.Info().
		Str("command", command).
		Int64("user_id", message.From.ID).
		Str("username", message.From.UserName).
		Msg("Received command")

	switch command {
	case "stats":
		b.handleStatsCommand(ctx, message)
	case "start", "help":
		b.handleHelpCommand(ctx, message)
	case "sync":
		b.handleSyncCommand(ctx, message)
	case "migrate_history":
		b.handleMigrateHistoryCommand(ctx, message)
	default:
		b.sendMessage(message.Chat.ID, "❓ Неизвестная команда. Используйте /help для списка команд.")
	}
}

// handleStatsCommand handles /stats command
func (b *Bot) handleStatsCommand(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	username := message.From.UserName
	firstName := message.From.FirstName

	// Get user stats
	stats, err := b.limiter.GetUserStats(ctx, userID, username, firstName)
	if err != nil {
		b.logger.Error().
			Err(err).
			Int64("user_id", userID).
			Msg("Failed to get user stats")
		b.sendErrorMessage(message.Chat.ID, "❌ Ошибка при получении статистики")
		return
	}

	// Format stats message
	statsMsg := fmt.Sprintf(
		"📊 *Статистика для %s*\n\n"+
			"🤖 *Gemini Pro (Thinking):*\n"+
			"   Использовано: %d/%d\n"+
			"   Осталось: %d\n\n"+
			"⚡ *Gemini Flash:*\n"+
			"   Использовано: %d/%d\n"+
			"   Осталось: %d\n\n"+
			"📈 *Всего запросов:* %d\n"+
			"⏰ *Сброс лимитов через:* %d ч.",
		firstName,
		stats.ProRequestsUsed, stats.ProRequestsLimit,
		stats.ProRequestsLimit-stats.ProRequestsUsed,
		stats.FlashRequestsUsed, stats.FlashRequestsLimit,
		stats.FlashRequestsLimit-stats.FlashRequestsUsed,
		stats.TotalRequests,
		stats.ResetsInHours,
	)

	b.sendMessage(message.Chat.ID, statsMsg)
}

// handleHelpCommand handles /help and /start commands
func (b *Bot) handleHelpCommand(ctx context.Context, message *tgbotapi.Message) {
	ragStatus := "отключен ❌"
	if b.config.RAG.Enabled {
		ragStatus = "включен ✅"
	}

	helpMsg := fmt.Sprintf(
		"👋 *Привет! Я бот с AI ассистентом*\n\n"+
			"*Как использовать:*\n"+
			"Просто упомяните меня (@%s) и задайте вопрос!\n\n"+
			"*Доступные команды:*\n"+
			"/stats - Посмотреть свою статистику\n"+
			"/sync - Запустить индексацию новых сообщений\n"+
			"/migrate_history - Инструкция по загрузке истории чата\n"+
			"/help - Показать это сообщение\n\n"+
			"*Лимиты:*\n"+
			"• Gemini Pro (думающая модель): %d запросов/день\n"+
			"• Gemini Flash (быстрая модель): %d запросов/день\n\n"+
			"*RAG (поиск по истории чата):* %s\n\n"+
			"Сначала используются запросы к Pro модели, затем к Flash.\n"+
			"Лимиты сбрасываются в полночь по московскому времени.",
		b.config.TelegramUsername,
		b.config.ProDailyLimit,
		b.config.FlashDailyLimit,
		ragStatus,
	)

	b.sendMessage(message.Chat.ID, helpMsg)
}

// handleMention processes messages where bot is mentioned
func (b *Bot) handleMention(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	username := message.From.UserName
	firstName := message.From.FirstName
	chatID := message.Chat.ID

	// Extract question text (remove bot mention)
	questionText := b.extractQuestion(message)
	if questionText == "" {
		b.sendMessage(chatID, "❓ Пожалуйста, задайте вопрос после упоминания.")
		return
	}

	// Check question length and truncate if needed
	questionRunes := []rune(questionText)
	if len(questionRunes) > MaxQuestionLength {
		b.logger.Warn().
			Int64("user_id", userID).
			Int("question_length", len(questionRunes)).
			Msg("Question too long, truncating")

		questionText = string(questionRunes[:MaxQuestionLength])

		// Notify user about truncation
		b.sendMessage(chatID, fmt.Sprintf(
			"⚠️ Ваш вопрос был обрезан до %d символов. Пожалуйста, формулируйте вопросы короче.",
			MaxQuestionLength,
		))
	}

	b.logger.Info().
		Int64("user_id", userID).
		Str("username", username).
		Int("question_length", len(questionRunes)).
		Msg("Processing mention")

	// Send typing action
	b.sendTypingAction(chatID)

	// Perform RAG search if enabled
	var ragContext string
	if b.config.RAG.Enabled {
		b.logger.Debug().
			Str("query", questionText[:min(50, len(questionText))]).
			Msg("Performing RAG search")

		ragResult, err := b.ragSearcher.Search(ctx, questionText, chatID)
		if err != nil {
			b.logger.Error().
				Err(err).
				Int64("user_id", userID).
				Msg("RAG search failed, continuing without context")
			// Continue without RAG context on error
			ragContext = ""
		} else {
			ragContext = ragResult.Context
			b.logger.Info().
				Int("results_count", ragResult.Count).
				Int64("user_id", userID).
				Msg("RAG search completed")
		}
	}

	// Check rate limits
	limitResult, err := b.limiter.CheckLimit(ctx, userID)
	if err != nil {
		b.logger.Error().
			Err(err).
			Int64("user_id", userID).
			Msg("Failed to check rate limit")
		b.sendErrorMessage(chatID, "❌ Ошибка при проверке лимитов")
		return
	}

	// If limit exceeded, send message and return
	if !limitResult.Allowed {
		b.sendMessage(chatID, limitResult.Message)
		return
	}

	// Create LLM request with RAG context
	llmReq := &models.LLMRequest{
		UserID:      userID,
		Username:    username,
		FirstName:   firstName,
		ChatID:      chatID,
		Text:        questionText,
		RAGContext:  ragContext,
		ModelType:   limitResult.ModelToUse,
		TimeoutSecs: b.config.GeminiTimeout,
	}

	// Generate response from LLM
	llmResp := b.llmClient.GenerateResponse(ctx, llmReq)

	// Check for errors
	if llmResp.Error != nil {
		b.logger.Error().
			Err(llmResp.Error).
			Int64("user_id", userID).
			Str("model", llmResp.ModelUsed).
			Msg("LLM request failed")

		// Don't increment usage if request failed
		b.sendErrorMessage(chatID, "❌ Извините, произошла ошибка при обработке вашего запроса. Попробуйте позже.")

		// Log failed request
		if err := b.storage.LogRequest(ctx, &models.RequestLog{
			UserID:          userID,
			Username:        username,
			FirstName:       firstName,
			ChatID:          chatID,
			RequestText:     questionText,
			ResponseText:    "",
			ModelUsed:       llmResp.ModelUsed,
			ResponseLength:  0,
			ExecutionTimeMs: llmResp.ExecutionTimeMs,
			ErrorMessage:    llmResp.Error.Error(),
			CreatedAt:       time.Now().UTC(),
		}); err != nil {
			b.logger.Error().
				Err(err).
				Int64("user_id", userID).
				Msg("Failed to log failed request, but continuing")
		}

		return
	}

	// Increment usage
	err = b.limiter.IncrementUsage(ctx, userID, limitResult.ModelToUse)
	if err != nil {
		b.logger.Error().
			Err(err).
			Int64("user_id", userID).
			Msg("Failed to increment usage")
		// Continue anyway, we already generated the response
	}

	// Log successful request
	// Note: We use UTC for database timestamps to maintain consistency
	// Rate limiter uses configured timezone (Moscow) for daily limits
	// This separation allows proper timezone-based limit resets while
	// keeping database timestamps in universal format
	if err := b.storage.LogRequest(ctx, &models.RequestLog{
		UserID:          userID,
		Username:        username,
		FirstName:       firstName,
		ChatID:          chatID,
		RequestText:     questionText,
		ResponseText:    llmResp.Text,
		ModelUsed:       llmResp.ModelUsed,
		ResponseLength:  llmResp.Length,
		ExecutionTimeMs: llmResp.ExecutionTimeMs,
		ErrorMessage:    "",
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		b.logger.Error().
			Err(err).
			Int64("user_id", userID).
			Msg("Failed to log successful request, but continuing")
	}

	// Determine model emoji
	modelEmoji := "⚡"
	if limitResult.ModelToUse == models.ModelPro {
		modelEmoji = "🤖"
	}

	// Send response
	responseMsg := fmt.Sprintf(
		"%s\n\n---\n%s _Модель: %s | Время: %dмс_",
		llmResp.Text,
		modelEmoji,
		string(limitResult.ModelToUse),
		llmResp.ExecutionTimeMs,
	)

	b.sendMessage(chatID, responseMsg)
}

// isMentioned checks if bot is mentioned in the message
func (b *Bot) isMentioned(message *tgbotapi.Message) bool {
	// Check entities for mentions
	for _, entity := range message.Entities {
		if entity.Type == "mention" {
			mention := message.Text[entity.Offset : entity.Offset+entity.Length]
			if strings.EqualFold(mention, "@"+b.config.TelegramUsername) {
				return true
			}
		}
	}

	// Also check if message text contains bot username
	return strings.Contains(strings.ToLower(message.Text), strings.ToLower("@"+b.config.TelegramUsername))
}

// extractQuestion extracts the question text from message, removing bot mention
func (b *Bot) extractQuestion(message *tgbotapi.Message) string {
	text := message.Text

	// Remove bot mention
	botMention := "@" + b.config.TelegramUsername
	text = strings.ReplaceAll(text, botMention, "")
	text = strings.ReplaceAll(text, strings.ToLower(botMention), "")

	// Trim whitespace
	text = strings.TrimSpace(text)

	return text
}

// saveChatMessage saves a message to chat_messages table for RAG
func (b *Bot) saveChatMessage(ctx context.Context, message *tgbotapi.Message) {
	// Skip if message has no text
	if message.Text == "" {
		return
	}

	// Create chat message
	chatMsg := &models.ChatMessage{
		MessageID:   int64(message.MessageID),
		UserID:      message.From.ID,
		Username:    message.From.UserName,
		FirstName:   message.From.FirstName,
		ChatID:      message.Chat.ID,
		MessageText: message.Text,
		Indexed:     false,
		CreatedAt:   message.Time(),
	}

	// Save to database
	if err := b.storage.SaveChatMessage(ctx, chatMsg); err != nil {
		b.logger.Error().
			Err(err).
			Int64("message_id", int64(message.MessageID)).
			Int64("user_id", message.From.ID).
			Msg("Failed to save chat message")
	} else {
		b.logger.Debug().
			Int64("message_id", int64(message.MessageID)).
			Int64("user_id", message.From.ID).
			Msg("Chat message saved successfully")
	}
}

// handleSyncCommand handles /sync command (manual RAG synchronization)
func (b *Bot) handleSyncCommand(ctx context.Context, message *tgbotapi.Message) {
	// TODO: Add admin check here
	// For now, anyone can trigger sync

	b.sendMessage(message.Chat.ID, "🔄 Запускаю синхронизацию RAG...\n\nЭто может занять несколько минут.")

	b.logger.Info().
		Int64("user_id", message.From.ID).
		Str("username", message.From.UserName).
		Msg("Manual RAG sync requested")

	// Run sync in background
	go b.runManualSync(context.Background(), message.Chat.ID, message.From.ID)
}

// runManualSync runs manual RAG synchronization
func (b *Bot) runManualSync(ctx context.Context, chatID, userID int64) {
	startTime := time.Now()

	// Get unindexed messages
	limit := 1000
	messages, err := b.storage.GetUnindexedMessages(ctx, limit)
	if err != nil {
		b.logger.Error().
			Err(err).
			Msg("Failed to get unindexed messages")
		b.sendMessage(chatID, "❌ Ошибка при получении неиндексированных сообщений")
		return
	}

	if len(messages) == 0 {
		b.sendMessage(chatID, "✅ Все сообщения уже проиндексированы!")
		return
	}

	b.logger.Info().
		Int("count", len(messages)).
		Msg("Processing unindexed messages")

	// Extract texts for embedding
	texts := make([]string, len(messages))
	ids := make([]int64, len(messages))
	for i, msg := range messages {
		texts[i] = msg.MessageText
		ids[i] = msg.ID
	}

	// Generate embeddings in batch
	embeddings, err := b.embeddingsClient.GenerateEmbeddingsBatch(ctx, texts)
	if err != nil {
		b.logger.Error().
			Err(err).
			Msg("Failed to generate embeddings")
		b.sendMessage(chatID, "❌ Ошибка при генерации эмбеддингов")
		return
	}

	// Update messages with embeddings
	updated, err := b.storage.BatchUpdateEmbeddings(ctx, ids, embeddings)
	if err != nil {
		b.logger.Error().
			Err(err).
			Msg("Failed to update embeddings")
		b.sendMessage(chatID, "❌ Ошибка при обновлении эмбеддингов")
		return
	}

	duration := time.Since(startTime)

	b.logger.Info().
		Int("updated", updated).
		Dur("duration", duration).
		Int64("user_id", userID).
		Msg("Manual sync completed")

	msg := fmt.Sprintf(
		"✅ Синхронизация завершена!\n\n"+
			"Проиндексировано сообщений: %d\n"+
			"Время выполнения: %.1f сек",
		updated,
		duration.Seconds(),
	)

	b.sendMessage(chatID, msg)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
