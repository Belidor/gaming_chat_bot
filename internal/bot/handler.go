package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/telegram-llm-bot/internal/models"
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
	// Only process messages from the configured group chat
	if message.Chat.ID != b.config.GroupChatID {
		b.logger.Debug().
			Int64("chat_id", message.Chat.ID).
			Int64("expected_chat_id", b.config.GroupChatID).
			Msg("Ignoring message from different chat")
		return
	}

	// Handle commands
	if message.IsCommand() {
		b.handleCommand(ctx, message)
		return
	}

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
	helpMsg := fmt.Sprintf(
		"👋 *Привет! Я бот с AI ассистентом*\n\n"+
			"*Как использовать:*\n"+
			"Просто упомяните меня (@%s) и задайте вопрос!\n\n"+
			"*Доступные команды:*\n"+
			"/stats - Посмотреть свою статистику\n"+
			"/help - Показать это сообщение\n\n"+
			"*Лимиты:*\n"+
			"• Gemini Pro (думающая модель): %d запросов/день\n"+
			"• Gemini Flash (быстрая модель): %d запросов/день\n\n"+
			"Сначала используются запросы к Pro модели, затем к Flash.\n"+
			"Лимиты сбрасываются в полночь по московскому времени.",
		b.config.TelegramUsername,
		b.config.ProDailyLimit,
		b.config.FlashDailyLimit,
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

	b.logger.Info().
		Int64("user_id", userID).
		Str("username", username).
		Str("question", questionText).
		Msg("Processing mention")

	// Send typing action
	b.sendTypingAction(chatID)

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

	// Create LLM request
	llmReq := &models.LLMRequest{
		UserID:      userID,
		Username:    username,
		FirstName:   firstName,
		ChatID:      chatID,
		Text:        questionText,
		ModelType:   limitResult.ModelToUse,
		MaxLength:   b.config.MaxResponseLen,
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
		_ = b.storage.LogRequest(ctx, &models.RequestLog{
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
		})
		
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
	_ = b.storage.LogRequest(ctx, &models.RequestLog{
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
	})

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
