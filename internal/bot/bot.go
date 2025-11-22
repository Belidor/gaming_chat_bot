package bot

import (
	"context"
	"fmt"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
	"github.com/telegram-llm-bot/internal/llm"
	"github.com/telegram-llm-bot/internal/models"
	"github.com/telegram-llm-bot/internal/rag"
	"github.com/telegram-llm-bot/internal/ratelimit"
	"github.com/telegram-llm-bot/internal/storage"
)

// Bot represents the Telegram bot
type Bot struct {
	api             *tgbotapi.BotAPI
	config          *models.BotConfig
	storage         *storage.Client
	llmClient       *llm.Client
	ragSearcher     *rag.Searcher
	limiter         *ratelimit.Limiter
	logger          zerolog.Logger
	wg              sync.WaitGroup // Tracks active handlers for graceful shutdown
	summaryCallback func(chatID int64) error
	syncCallback    func() error
}

// New creates a new bot instance
func New(
	config *models.BotConfig,
	storage *storage.Client,
	llmClient *llm.Client,
	ragSearcher *rag.Searcher,
	limiter *ratelimit.Limiter,
	logger zerolog.Logger,
) (*Bot, error) {
	// Create Telegram bot API client
	api, err := tgbotapi.NewBotAPI(config.TelegramToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	// Set debug mode based on log level
	api.Debug = config.LogLevel == "debug"

	logger.Info().
		Str("username", api.Self.UserName).
		Int64("id", api.Self.ID).
		Msg("Telegram bot authorized")

	return &Bot{
		api:         api,
		config:      config,
		storage:     storage,
		llmClient:   llmClient,
		ragSearcher: ragSearcher,
		limiter:     limiter,
		logger:      logger.With().Str("component", "bot").Logger(),
	}, nil
}

// Start starts the bot
func (b *Bot) Start(ctx context.Context) error {
	b.logger.Info().Msg("Starting bot...")

	// Configure update settings
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Get updates channel
	updates := b.api.GetUpdatesChan(u)

	b.logger.Info().Msg("Bot started, waiting for messages...")

	// Process updates
	for {
		select {
		case <-ctx.Done():
			b.logger.Info().Msg("Shutting down bot...")
			b.api.StopReceivingUpdates()

			// Wait for all active handlers to complete
			b.logger.Info().Msg("Waiting for active handlers to complete...")
			b.wg.Wait()
			b.logger.Info().Msg("All handlers completed")

			return nil

		case update := <-updates:
			// Track this handler in WaitGroup
			b.wg.Add(1)
			// Process update in a goroutine to not block
			go func(upd tgbotapi.Update) {
				defer b.wg.Done()
				b.handleUpdate(ctx, upd)
			}(update)
		}
	}
}

// Stop stops the bot
func (b *Bot) Stop() {
	b.logger.Info().Msg("Stopping bot...")
	b.api.StopReceivingUpdates()
}

// GetUsername returns bot username
func (b *Bot) GetUsername() string {
	return b.api.Self.UserName
}

// SendDailySummary sends a daily summary message to a chat
func (b *Bot) SendDailySummary(chatID int64, summaryText string) error {
	b.logger.Info().
		Int64("chat_id", chatID).
		Msg("Sending daily summary")

	msg := tgbotapi.NewMessage(chatID, summaryText)
	msg.ParseMode = "Markdown"

	_, err := b.api.Send(msg)
	if err != nil {
		b.logger.Error().
			Err(err).
			Int64("chat_id", chatID).
			Msg("Failed to send daily summary")
		return fmt.Errorf("failed to send daily summary: %w", err)
	}

	b.logger.Info().
		Int64("chat_id", chatID).
		Msg("Daily summary sent successfully")

	return nil
}

// SetSummaryCallback sets the callback function for manual summary generation
func (b *Bot) SetSummaryCallback(callback func(chatID int64) error) {
	b.summaryCallback = callback
}

// SetSyncCallback sets the callback function for manual RAG sync
func (b *Bot) SetSyncCallback(callback func() error) {
	b.syncCallback = callback
}

// saveChatMessage saves a chat message to the database for RAG and summaries
func (b *Bot) saveChatMessage(ctx context.Context, message *tgbotapi.Message) {
	// Skip if no text
	if message.Text == "" {
		return
	}

	// Create chat message model
	chatMsg := &models.ChatMessage{
		MessageID:   int64(message.MessageID),
		UserID:      message.From.ID,
		Username:    message.From.UserName,
		FirstName:   message.From.FirstName,
		ChatID:      message.Chat.ID,
		MessageText: message.Text,
		CreatedAt:   time.Unix(int64(message.Date), 0).UTC(),
	}

	// Save to database (non-blocking, log errors but don't fail)
	if err := b.storage.SaveChatMessage(ctx, chatMsg); err != nil {
		b.logger.Error().
			Err(err).
			Int64("message_id", int64(message.MessageID)).
			Int64("chat_id", message.Chat.ID).
			Int64("user_id", message.From.ID).
			Msg("Failed to save chat message")
	} else {
		b.logger.Debug().
			Int64("message_id", int64(message.MessageID)).
			Int64("chat_id", message.Chat.ID).
			Int64("user_id", message.From.ID).
			Msg("Chat message saved for RAG/summaries")
	}
}
