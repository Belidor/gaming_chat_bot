package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/telegram-llm-bot/internal/embeddings"
	"github.com/telegram-llm-bot/internal/models"
	"github.com/telegram-llm-bot/internal/storage"
)

// SyncJob handles RAG synchronization
type SyncJob struct {
	storage          *storage.Client
	embeddingsClient *embeddings.Client
	batchSize        int
	maxMessages      int
	logger           zerolog.Logger
}

// NewSyncJob creates a new sync job
func NewSyncJob(
	storage *storage.Client,
	embeddingsClient *embeddings.Client,
	batchSize int,
	maxMessages int,
	logger zerolog.Logger,
) *SyncJob {
	return &SyncJob{
		storage:          storage,
		embeddingsClient: embeddingsClient,
		batchSize:        batchSize,
		maxMessages:      maxMessages,
		logger:           logger.With().Str("component", "sync_job").Logger(),
	}
}

// Run executes the sync job
func (j *SyncJob) Run(ctx context.Context) error {
	startTime := time.Now()

	j.logger.Info().Msg("Starting RAG sync job")

	// Get unindexed messages
	messages, err := j.storage.GetUnindexedMessages(ctx, j.maxMessages)
	if err != nil {
		return fmt.Errorf("failed to get unindexed messages: %w", err)
	}

	if len(messages) == 0 {
		j.logger.Info().Msg("No unindexed messages found")
		return nil
	}

	j.logger.Info().
		Int("count", len(messages)).
		Msg("Found unindexed messages, starting processing")

	// Process in batches
	totalProcessed := 0
	for i := 0; i < len(messages); i += j.batchSize {
		end := i + j.batchSize
		if end > len(messages) {
			end = len(messages)
		}

		batch := messages[i:end]
		processed, err := j.processBatch(ctx, batch)
		if err != nil {
			j.logger.Error().
				Err(err).
				Int("batch_start", i).
				Int("batch_end", end).
				Msg("Failed to process batch, continuing with next")
			continue
		}

		totalProcessed += processed

		j.logger.Info().
			Int("batch_start", i).
			Int("batch_end", end).
			Int("processed", processed).
			Msg("Batch processed successfully")
	}

	duration := time.Since(startTime)

	j.logger.Info().
		Int("total_processed", totalProcessed).
		Dur("duration", duration).
		Msg("RAG sync job completed")

	return nil
}

// processBatch processes a batch of messages
func (j *SyncJob) processBatch(ctx context.Context, messages []*models.ChatMessage) (int, error) {
	if len(messages) == 0 {
		return 0, nil
	}

	// Extract texts and IDs
	texts := make([]string, len(messages))
	ids := make([]int64, len(messages))
	for i, msg := range messages {
		texts[i] = msg.MessageText
		ids[i] = msg.ID
	}

	// Generate embeddings
	embeddings, err := j.embeddingsClient.GenerateEmbeddingsBatch(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("failed to generate embeddings: %w", err)
	}

	// Update messages with embeddings
	updated, err := j.storage.BatchUpdateEmbeddings(ctx, ids, embeddings)
	if err != nil {
		return 0, fmt.Errorf("failed to update embeddings: %w", err)
	}

	return updated, nil
}
