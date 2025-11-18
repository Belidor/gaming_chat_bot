package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/telegram-llm-bot/internal/bot"
	"github.com/telegram-llm-bot/internal/config"
	"github.com/telegram-llm-bot/internal/llm"
	"github.com/telegram-llm-bot/internal/ratelimit"
	"github.com/telegram-llm-bot/internal/storage"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Setup logger
	logger := setupLogger(cfg.LogLevel, cfg.Environment)
	logger.Info().
		Str("environment", cfg.Environment).
		Str("timezone", cfg.Timezone).
		Int("pro_limit", cfg.ProDailyLimit).
		Int("flash_limit", cfg.FlashDailyLimit).
		Msg("Starting Telegram LLM Bot")

	// Create context that listens for termination signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	if err := storageClient.Ping(ctx); err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to Supabase")
	}
	logger.Info().Msg("Supabase connection successful")

	// Initialize LLM client
	logger.Info().Msg("Initializing Gemini LLM client...")
	llmClient := llm.NewClient(cfg.GeminiAPIKey, cfg.GeminiTimeout, logger)

	// Initialize rate limiter
	logger.Info().Msg("Initializing rate limiter...")
	limiter, err := ratelimit.NewLimiter(
		storageClient,
		cfg.Timezone,
		cfg.ProDailyLimit,
		cfg.FlashDailyLimit,
		logger,
	)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create rate limiter")
	}

	// Initialize bot
	logger.Info().Msg("Initializing Telegram bot...")
	telegramBot, err := bot.New(cfg, storageClient, llmClient, limiter, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create bot")
	}

	logger.Info().
		Str("username", telegramBot.GetUsername()).
		Int64("group_chat_id", cfg.GroupChatID).
		Msg("Bot initialized successfully")

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Start bot in a goroutine
	botErrChan := make(chan error, 1)
	go func() {
		if err := telegramBot.Start(ctx); err != nil {
			botErrChan <- err
		}
	}()

	logger.Info().Msg("Bot is running. Press Ctrl+C to stop.")

	// Wait for termination signal or bot error
	select {
	case sig := <-sigChan:
		logger.Info().Str("signal", sig.String()).Msg("Received termination signal")
	case err := <-botErrChan:
		logger.Error().Err(err).Msg("Bot stopped with error")
	}

	// Graceful shutdown
	logger.Info().Msg("Initiating graceful shutdown...")
	cancel()

	// Give the bot some time to finish processing
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	// Wait for shutdown or timeout
	select {
	case <-shutdownCtx.Done():
		if shutdownCtx.Err() == context.DeadlineExceeded {
			logger.Warn().Msg("Shutdown timeout exceeded, forcing exit")
		}
	case <-time.After(2 * time.Second):
		logger.Info().Msg("Graceful shutdown completed")
	}

	logger.Info().Msg("Bot stopped")
}

// setupLogger configures and returns a zerolog logger
func setupLogger(level, environment string) zerolog.Logger {
	// Parse log level
	logLevel, err := zerolog.ParseLevel(level)
	if err != nil {
		logLevel = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(logLevel)

	// Configure output format
	var logger zerolog.Logger
	if environment == "development" {
		// Pretty console output for development
		logger = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}).With().Timestamp().Caller().Logger()
	} else {
		// JSON output for production
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}

	return logger
}
