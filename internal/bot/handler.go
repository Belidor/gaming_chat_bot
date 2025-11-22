package bot

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

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

	// Save ALL messages from allowed chats to database for RAG and summaries
	// This is critical for the RAG system and daily summaries to work
	if message.Text != "" && message.From != nil {
		b.saveChatMessage(ctx, message)
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
	case "summary":
		b.handleSummaryCommand(ctx, message)
	case "sync":
		b.handleSyncCommand(ctx, message)
	case "draw":
		b.handleDrawCommand(ctx, message)
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
			"/draw <запрос> - Сгенерировать изображение по описанию\n"+
			"/summary - Сгенерировать саммари за вчерашний день\n"+
			"/sync - Запустить синхронизацию RAG (индексация сообщений)\n"+
			"/help - Показать это сообщение\n\n"+
			"*Лимиты:*\n"+
			"• Gemini Pro (думающая модель): %d запросов/день\n"+
			"• Gemini Flash (быстрая модель): %d запросов/день\n"+
			"• Генерация изображений: %d генераций/день\n\n"+
			"Сначала используются запросы к Pro модели, затем к Flash.\n"+
			"Лимиты сбрасываются в полночь по московскому времени.\n\n"+
			"*Примеры:*\n"+
			"• /draw красивый закат над океаном\n"+
			"• /draw кот в космосе в стиле киберпанк\n\n"+
			"*Автоматические задачи:*\n"+
			"• 03:00 МСК - Синхронизация RAG (индексация embeddings)\n"+
			"• 07:00 МСК - Ежедневное саммари",
		b.config.TelegramUsername,
		b.config.ProDailyLimit,
		b.config.FlashDailyLimit,
		b.config.ImageGenerationDailyLimitPerUser,
	)

	b.sendMessage(message.Chat.ID, helpMsg)
}

// handleSummaryCommand handles /summary command - generates summary for yesterday
func (b *Bot) handleSummaryCommand(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID

	// Only allow in allowed chats
	if !b.config.IsAllowedChat(chatID) {
		b.sendMessage(chatID, "❌ Эта команда доступна только в разрешенных чатах.")
		return
	}

	b.logger.Info().
		Int64("chat_id", chatID).
		Int64("user_id", message.From.ID).
		Str("username", message.From.UserName).
		Msg("Manual summary generation requested")

	// Send "generating" message
	b.sendMessage(chatID, "⏳ Генерирую саммари за вчерашний день...")

	// Trigger summary generation callback if available
	if b.summaryCallback != nil {
		if err := b.summaryCallback(chatID); err != nil {
			b.logger.Error().
				Err(err).
				Int64("chat_id", chatID).
				Msg("Failed to generate manual summary")
			b.sendMessage(chatID, "❌ Ошибка при генерации саммари. Попробуйте позже.")
			return
		}
	} else {
		b.sendMessage(chatID, "❌ Функция саммари не настроена.")
	}
}

// handleSyncCommand handles /sync command - manual RAG synchronization
func (b *Bot) handleSyncCommand(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID

	// Only allow in allowed chats
	if !b.config.IsAllowedChat(chatID) {
		b.sendMessage(chatID, "❌ Эта команда доступна только в разрешенных чатах.")
		return
	}

	b.logger.Info().
		Int64("chat_id", chatID).
		Int64("user_id", message.From.ID).
		Str("username", message.From.UserName).
		Msg("Manual RAG sync requested")

	// Send "starting" message
	b.sendMessage(chatID, "🔄 Запускаю синхронизацию RAG...\n\nЭто может занять несколько минут.")

	// Trigger sync callback if available
	if b.syncCallback != nil {
		// Run in goroutine to not block
		go func() {
			if err := b.syncCallback(); err != nil {
				b.logger.Error().
					Err(err).
					Int64("chat_id", chatID).
					Msg("Failed to run manual sync")
				b.sendMessage(chatID, "❌ Ошибка при синхронизации. Попробуйте позже.")
			} else {
				b.sendMessage(chatID, "✅ Синхронизация завершена успешно!")
			}
		}()
	} else {
		b.sendMessage(chatID, "❌ Функция синхронизации не настроена.")
	}
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

	// Perform RAG search for relevant context
	var ragContext string
	ragResult, err := b.ragSearcher.Search(ctx, questionText, chatID)
	if err != nil {
		b.logger.Warn().
			Err(err).
			Int64("user_id", userID).
			Int64("chat_id", chatID).
			Msg("RAG search failed, continuing without context")
		// Continue without RAG context - don't fail the request
		ragContext = ""
	} else {
		ragContext = ragResult.Context
		b.logger.Info().
			Int64("user_id", userID).
			Int64("chat_id", chatID).
			Int("rag_results_count", ragResult.Count).
			Msg("RAG search completed successfully")
	}

	// Create LLM request
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
	username := strings.ToLower("@" + b.config.TelegramUsername)
	for _, entity := range message.Entities {
		switch entity.Type {
		case "mention":
			mention := extractEntityText(message.Text, entity.Offset, entity.Length)
			if strings.EqualFold(mention, username) {
				return true
			}
		case "text_mention":
			if entity.User != nil {
				if entity.User.UserName != "" && strings.EqualFold("@"+entity.User.UserName, username) {
					return true
				}
				if entity.User.ID == b.api.Self.ID {
					return true
				}
			}
		}
	}

	// Fallback check to handle cases where Telegram didn't tag entities
	return strings.Contains(strings.ToLower(message.Text), username)
}

// handleDrawCommand handles /draw command - generates an image from text prompt
func (b *Bot) handleDrawCommand(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	userID := message.From.ID
	username := message.From.UserName
	firstName := message.From.FirstName

	// Extract prompt text after /draw command
	prompt := strings.TrimSpace(message.CommandArguments())

	// Validate prompt is not empty
	if prompt == "" {
		b.sendMessage(chatID, "Укажите описание изображения. Пример: /draw красивый закат над океаном")
		return
	}

	// Validate prompt length (max 500 characters as per spec)
	if len([]rune(prompt)) > 500 {
		b.sendMessage(chatID, "⚠️ Описание слишком длинное. Максимум 500 символов.")
		return
	}

	b.logger.Info().
		Int64("user_id", userID).
		Str("username", username).
		Int("prompt_length", len([]rune(prompt))).
		Msg("Processing /draw command")

	// Get current date in configured timezone for limit checking
	loc, err := time.LoadLocation(b.config.Timezone)
	if err != nil {
		b.logger.Error().Err(err).Msg("Failed to load timezone, using UTC")
		loc = time.UTC
	}
	currentDate := time.Now().In(loc).Format("2006-01-02")

	// Check image generation limits
	allowed, remaining, err := b.storage.CheckImageGenerationLimit(ctx, userID, chatID, currentDate, b.config)
	if err != nil {
		b.logger.Error().
			Err(err).
			Int64("user_id", userID).
			Msg("Failed to check image generation limit")
		b.sendErrorMessage(chatID, "❌ Ошибка при проверке лимитов")
		return
	}

	if !allowed {
		b.sendMessage(chatID, fmt.Sprintf(
			"❌ Вы исчерпали дневной лимит генераций (%d/день). Попробуйте завтра.",
			b.config.ImageGenerationDailyLimitPerUser,
		))
		return
	}

	// Send "generating" message
	b.sendMessage(chatID, "🎨 Генерирую изображение...")
	b.sendTypingAction(chatID)

	// Generate image
	imageData, err := b.llmClient.GenerateImage(ctx, prompt)
	if err != nil {
		b.logger.Error().
			Err(err).
			Int64("user_id", userID).
			Str("prompt", prompt).
			Msg("Failed to generate image")

		b.sendErrorMessage(chatID, "⚠️ Сервис генерации временно недоступен. Попробуйте позже.")
		return
	}

	// Record usage
	if err := b.storage.RecordImageGeneration(ctx, userID, chatID, currentDate); err != nil {
		b.logger.Error().
			Err(err).
			Int64("user_id", userID).
			Msg("Failed to record image generation")
		// Continue anyway, we already generated the image
	}

	// Send image to user
	photoConfig := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{
		Name:  "generated_image.jpg",
		Bytes: imageData,
	})

	// Add caption with remaining count
	remaining-- // Decrement since we just used one
	photoConfig.Caption = fmt.Sprintf("✨ Осталось генераций сегодня: %d", remaining)

	_, err = b.api.Send(photoConfig)
	if err != nil {
		b.logger.Error().
			Err(err).
			Int64("user_id", userID).
			Msg("Failed to send generated image")
		b.sendErrorMessage(chatID, "❌ Не удалось отправить изображение")
		return
	}

	b.logger.Info().
		Int64("user_id", userID).
		Str("username", username).
		Str("first_name", firstName).
		Int("remaining", remaining).
		Msg("Image generated and sent successfully")
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

func extractEntityText(text string, offset, length int) string {
	startByte, endByte := utf16RangeToByteRange(text, offset, length)
	if startByte == -1 || endByte == -1 || startByte >= endByte || startByte >= len(text) {
		return ""
	}
	if endByte > len(text) {
		endByte = len(text)
	}
	return text[startByte:endByte]
}

func utf16RangeToByteRange(s string, offset, length int) (int, int) {
	if offset < 0 || length < 0 {
		return -1, -1
	}
	targetStart := offset
	targetEnd := offset + length

	var (
		currentUTF16 int
		byteIndex    int
		startByte    = -1
		endByte      = -1
	)

	for _, r := range s {
		runeLen := utf8.RuneLen(r)
		units := 1
		if r >= 0x10000 {
			units = 2
		}

		if startByte == -1 && currentUTF16 >= targetStart {
			startByte = byteIndex
		}
		if startByte != -1 && endByte == -1 && currentUTF16 >= targetEnd {
			endByte = byteIndex
			break
		}

		currentUTF16 += units
		byteIndex += runeLen
	}

	if startByte == -1 {
		if targetStart == currentUTF16 {
			startByte = byteIndex
		} else {
			return -1, -1
		}
	}

	if endByte == -1 {
		if targetEnd <= currentUTF16 {
			endByte = byteIndex
		} else {
			endByte = len(s)
		}
	}

	return startByte, endByte
}
