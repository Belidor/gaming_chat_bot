package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/telegram-llm-bot/internal/config"
	"github.com/telegram-llm-bot/internal/embeddings"
	"github.com/telegram-llm-bot/internal/storage"
)

func main() {
	// Parse flags
	batchSize := flag.Int("batch", 100, "Batch size for processing")
	limit := flag.Int("limit", 0, "Maximum number of messages to process (0 = no limit)")
	dryRun := flag.Bool("dry-run", false, "Dry run mode (don't save to database)")
	flag.Parse()

	// Setup logging
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "15:04:05"})

	log.Info().Msg("Starting embeddings generation script")

	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Warn().Err(err).Msg("No .env file found, using environment variables")
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	log.Info().
		Bool("dry_run", *dryRun).
		Int("batch_size", *batchSize).
		Int("limit", *limit).
		Msg("Configuration loaded")

	// Initialize storage client
	storageClient, err := storage.NewClient(
		cfg.SupabaseURL,
		cfg.SupabaseKey,
		cfg.SupabaseTimeout,
		log.Logger,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize storage client")
	}
	log.Info().Msg("Storage client initialized")

	// Initialize embeddings client
	embeddingsClient := embeddings.NewClient(
		cfg.GeminiAPIKey,
		cfg.RAG.EmbeddingsModel,
		cfg.RAG.EmbeddingsBatchSize,
		time.Duration(cfg.GeminiTimeout)*time.Second,
		log.Logger,
	)
	log.Info().Msg("Embeddings client initialized")

	ctx := context.Background()
	totalProcessed := 0
	totalUpdated := 0

	log.Info().Msg("Starting embeddings generation...")

	for {
		// Calculate remaining messages to process
		remaining := *limit - totalProcessed
		if *limit > 0 && remaining <= 0 {
			log.Info().
				Int("limit", *limit).
				Int("processed", totalProcessed).
				Msg("Reached processing limit")
			break
		}

		// Adjust batch size if we're close to the limit
		currentBatchSize := *batchSize
		if *limit > 0 && remaining < currentBatchSize {
			currentBatchSize = remaining
		}

		// Get unindexed messages
		messages, err := storageClient.GetUnindexedMessages(ctx, currentBatchSize)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get unindexed messages")
			break
		}

		if len(messages) == 0 {
			log.Info().Msg("No more unindexed messages")
			break
		}

		log.Info().
			Int("batch_size", len(messages)).
			Int("total_processed", totalProcessed).
			Msg("Processing batch")

		// Extract texts and IDs
		var texts []string
		var ids []int64
		for _, msg := range messages {
			texts = append(texts, msg.MessageText)
			ids = append(ids, msg.ID)
		}

		// Generate embeddings
		log.Info().Int("count", len(texts)).Msg("Generating embeddings...")
		generatedEmbeddings, err := embeddingsClient.GenerateEmbeddingsBatch(ctx, texts)
		if err != nil {
			log.Error().Err(err).Msg("Failed to generate embeddings")
			break
		}

		log.Info().
			Int("count", len(generatedEmbeddings)).
			Int("dimension", len(generatedEmbeddings[0])).
			Msg("Embeddings generated successfully")

		if *dryRun {
			log.Info().Msg("Dry run mode: skipping database update")
			totalProcessed += len(messages)
			
			// Show sample
			if len(messages) > 0 {
				log.Info().
					Int64("id", messages[0].ID).
					Str("text", messages[0].MessageText[:min(50, len(messages[0].MessageText))]).
					Int("embedding_dim", len(generatedEmbeddings[0])).
					Msg("Sample message")
			}
		} else {
			// Update database
			log.Info().Int("count", len(ids)).Msg("Updating database...")
			updated, err := storageClient.BatchUpdateEmbeddings(ctx, ids, generatedEmbeddings)
			if err != nil {
				log.Error().Err(err).Msg("Failed to update embeddings")
				break
			}

			log.Info().
				Int("updated", updated).
				Int("expected", len(ids)).
				Msg("Batch update completed")

			totalProcessed += len(messages)
			totalUpdated += updated

			if updated != len(ids) {
				log.Warn().
					Int("updated", updated).
					Int("expected", len(ids)).
					Msg("Not all messages were updated")
			}
		}

		// Small delay to avoid rate limiting
		time.Sleep(100 * time.Millisecond)
	}

	// Summary
	duration := time.Since(time.Now())
	log.Info().
		Int("total_processed", totalProcessed).
		Int("total_updated", totalUpdated).
		Dur("duration", duration).
		Msg("Embeddings generation completed")

	if !*dryRun {
		// Get statistics
		stats, err := storageClient.GetRAGStatistics(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get statistics")
		} else {
			log.Info().
				Interface("statistics", stats).
				Msg("Final statistics")
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

