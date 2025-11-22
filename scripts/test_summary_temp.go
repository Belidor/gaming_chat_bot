package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/telegram-llm-bot/internal/config"
	"github.com/telegram-llm-bot/internal/scheduler"
	"github.com/telegram-llm-bot/internal/storage"
	"github.com/telegram-llm-bot/internal/summary"
)

func main() {
	// Setup logger with pretty console output
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	logger := zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}).With().Timestamp().Caller().Logger()

	logger.Info().Msg("Starting summary test script")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Get chat ID from command line or use first allowed chat
	var chatID int64
	if len(os.Args) > 1 {
		_, err := fmt.Sscanf(os.Args[1], "%d", &chatID)
		if err != nil {
			logger.Fatal().Err(err).Msg("Invalid chat ID provided")
		}
	} else if len(cfg.AllowedChatIDs) > 0 {
		chatID = cfg.AllowedChatIDs[0]
		logger.Info().
			Int64("chat_id", chatID).
			Msg("Using first allowed chat ID from config")
	} else {
		logger.Fatal().Msg("No chat ID provided and no allowed chats in config")
	}

	// Initialize storage client
	logger.Info().Msg("Initializing Supabase client...")
	storageClient, err := storage.NewClient(
		cfg.SupabaseURL,
		cfg.SupabaseKey,
		cfg.SupabaseTimeout,
		logger,
	)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create storage client")
	}

	// Ping Supabase to verify connection
	ctx := context.Background()
	if err := storageClient.Ping(ctx); err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to Supabase")
	}
	logger.Info().Msg("Supabase connection successful")

	// Initialize summary generator
	logger.Info().Msg("Initializing summary generator...")
	summaryGenerator := summary.NewGenerator(cfg.GeminiAPIKey, cfg, logger)
	defer func() {
		if err := summaryGenerator.Close(); err != nil {
			logger.Error().Err(err).Msg("Failed to close summary generator")
		}
	}()

	// Initialize scheduler (without sync job and callback for testing)
	logger.Info().Msg("Initializing scheduler...")
	summaryScheduler, err := scheduler.NewScheduler(
		storageClient,
		summaryGenerator,
		cfg,
		func(chatID int64, summaryText string) error {
			logger.Info().
				Int64("chat_id", chatID).
				Str("summary", summaryText).
				Msg("Summary would be sent to chat (not actually sending in test mode)")
			return nil
		},
		nil, // no sync job
		logger,
	)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create scheduler")
	}

	// Generate summary for yesterday
	logger.Info().
		Int64("chat_id", chatID).
		Msg("Generating summary for yesterday...")

	if err := summaryScheduler.GenerateSummaryForYesterday(ctx, chatID); err != nil {
		logger.Fatal().Err(err).Msg("Failed to generate summary")
	}

	logger.Info().Msg("Summary generation completed successfully")
}

