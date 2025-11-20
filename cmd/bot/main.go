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
	"github.com/telegram-llm-bot/internal/embeddings"
	"github.com/telegram-llm-bot/internal/llm"
	"github.com/telegram-llm-bot/internal/rag"
	"github.com/telegram-llm-bot/internal/ratelimit"
	"github.com/telegram-llm-bot/internal/scheduler"
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
		Bool("rag_enabled", cfg.RAG.Enabled).
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
	llmClient := llm.NewClient(cfg.GeminiAPIKey, cfg.GeminiTimeout, cfg, logger)
	defer func() {
		if err := llmClient.Close(); err != nil {
			logger.Error().Err(err).Msg("Failed to close LLM client")
		}
	}()

	// Initialize embeddings client (for RAG)
	logger.Info().Msg("Initializing embeddings client...")
	embeddingsClient := embeddings.NewClient(
		cfg.GeminiAPIKey,
		cfg.RAG.EmbeddingsModel,
		cfg.RAG.EmbeddingsBatchSize,
		time.Duration(cfg.GeminiTimeout)*time.Second,
		logger,
	)
	defer func() {
		if err := embeddingsClient.Close(); err != nil {
			logger.Error().Err(err).Msg("Failed to close embeddings client")
		}
	}()

	// Initialize RAG searcher
	logger.Info().Msg("Initializing RAG searcher...")
	ragSearcher := rag.NewSearcher(
		storageClient,
		embeddingsClient,
		cfg.RAG,
		logger,
	)

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
	telegramBot, err := bot.New(cfg, storageClient, llmClient, embeddingsClient, ragSearcher, limiter, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create bot")
	}

	logger.Info().
		Str("username", telegramBot.GetUsername()).
		Interface("allowed_chat_ids", cfg.AllowedChatIDs).
		Msg("Bot initialized successfully")

	// Initialize scheduler for RAG sync (if enabled)
	var sched *scheduler.Scheduler
	if cfg.RAG.Enabled {
		logger.Info().Msg("Initializing RAG scheduler...")
		syncJob := scheduler.NewSyncJob(
			storageClient,
			embeddingsClient,
			cfg.RAG.EmbeddingsBatchSize,
			1000, // max messages per sync
			logger,
		)
		sched = scheduler.NewScheduler(syncJob, logger)

		// Start scheduler in background
		go func() {
			if err := sched.Start(ctx); err != nil {
				logger.Error().Err(err).Msg("Scheduler stopped with error")
			}
		}()
		logger.Info().Msg("RAG scheduler started (nightly sync at 03:00 MSK)")
	}

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

	// Stop scheduler if running
	if sched != nil {
		logger.Info().Msg("Stopping scheduler...")
		sched.Stop()
	}

	// Give the bot some time to finish processing
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Create a channel to signal shutdown complete
	done := make(chan struct{})
	go func() {
		telegramBot.Stop() // This will wait for WaitGroup internally
		close(done)
	}()

	// Wait for shutdown or timeout
	select {
	case <-shutdownCtx.Done():
		logger.Warn().Msg("Shutdown timeout exceeded, some requests may be lost")
	case <-done:
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
