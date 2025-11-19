package bot

import (
	"context"
	"fmt"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
	"github.com/telegram-llm-bot/internal/llm"
	"github.com/telegram-llm-bot/internal/models"
	"github.com/telegram-llm-bot/internal/ratelimit"
	"github.com/telegram-llm-bot/internal/storage"
)

// Bot represents the Telegram bot
type Bot struct {
	api       *tgbotapi.BotAPI
	config    *models.BotConfig
	storage   *storage.Client
	llmClient *llm.Client
	limiter   *ratelimit.Limiter
	logger    zerolog.Logger
	wg        sync.WaitGroup // Tracks active handlers for graceful shutdown
}

// New creates a new bot instance
func New(
	config *models.BotConfig,
	storage *storage.Client,
	llmClient *llm.Client,
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
		api:       api,
		config:    config,
		storage:   storage,
		llmClient: llmClient,
		limiter:   limiter,
		logger:    logger.With().Str("component", "bot").Logger(),
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
