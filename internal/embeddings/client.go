package embeddings

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/rs/zerolog"
	"google.golang.org/api/option"
)

// Client represents a Gemini Embeddings client
type Client struct {
	apiKey      string
	model       string
	batchSize   int
	timeout     time.Duration
	logger      zerolog.Logger
	genaiClient *genai.Client
	mu          sync.Mutex
}

// NewClient creates a new Gemini Embeddings client
func NewClient(apiKey, model string, batchSize int, timeout time.Duration, logger zerolog.Logger) *Client {
	return &Client{
		apiKey:    apiKey,
		model:     model,
		batchSize: batchSize,
		timeout:   timeout,
		logger:    logger.With().Str("component", "embeddings").Logger(),
	}
}

// getClient returns or creates a genai client (thread-safe)
func (c *Client) getClient(ctx context.Context) (*genai.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.genaiClient != nil {
		return c.genaiClient, nil
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(c.apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	c.genaiClient = client
	c.logger.Info().Msg("Embeddings client created and cached")
	return c.genaiClient, nil
}

// Close closes the embeddings client and releases resources
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.genaiClient != nil {
		err := c.genaiClient.Close()
		c.genaiClient = nil
		if err != nil {
			c.logger.Error().Err(err).Msg("Failed to close embeddings client")
			return err
		}
		c.logger.Info().Msg("Embeddings client closed")
	}
	return nil
}

// GenerateEmbedding generates embedding for a single text
func (c *Client) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Use batch method with single item for consistency
	embeddings, err := c.GenerateEmbeddingsBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding generated")
	}

	return embeddings[0], nil
}

// GenerateEmbeddingsBatch generates embeddings for multiple texts
// Automatically splits into batches if needed
func (c *Client) GenerateEmbeddingsBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	// Get or create client
	client, err := c.getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get embeddings client: %w", err)
	}

	// Get embedding model
	em := client.EmbeddingModel(c.model)

	// If texts fit in one batch, process directly
	if len(texts) <= c.batchSize {
		return c.processBatch(ctx, em, texts)
	}

	// Split into multiple batches
	c.logger.Info().
		Int("total_texts", len(texts)).
		Int("batch_size", c.batchSize).
		Msg("Processing embeddings in batches")

	var allEmbeddings [][]float32
	for i := 0; i < len(texts); i += c.batchSize {
		end := i + c.batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		embeddings, err := c.processBatch(ctx, em, batch)
		if err != nil {
			return nil, fmt.Errorf("failed to process batch %d-%d: %w", i, end, err)
		}

		allEmbeddings = append(allEmbeddings, embeddings...)

		c.logger.Debug().
			Int("batch_start", i).
			Int("batch_end", end).
			Int("embeddings_count", len(embeddings)).
			Msg("Batch processed successfully")
	}

	c.logger.Info().
		Int("total_embeddings", len(allEmbeddings)).
		Msg("All batches processed successfully")

	return allEmbeddings, nil
}

// processBatch processes a single batch of texts
func (c *Client) processBatch(ctx context.Context, em *genai.EmbeddingModel, texts []string) ([][]float32, error) {
	startTime := time.Now()

	// Retry logic
	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			c.logger.Warn().
				Int("attempt", attempt+1).
				Dur("backoff", backoff).
				Msg("Retrying embeddings generation")

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		// Prepare batch request
		batch := em.NewBatch()
		for _, text := range texts {
			batch.AddContent(genai.Text(text))
		}

		// Generate embeddings
		result, err := em.BatchEmbedContents(ctx, batch)
		if err != nil {
			lastErr = err
			c.logger.Error().
				Err(err).
				Int("attempt", attempt+1).
				Int("batch_size", len(texts)).
				Msg("Failed to generate embeddings")
			continue
		}

		// Extract embeddings
		embeddings := make([][]float32, 0, len(result.Embeddings))
		for _, emb := range result.Embeddings {
			if emb != nil && len(emb.Values) > 0 {
				embeddings = append(embeddings, emb.Values)
			} else {
				return nil, fmt.Errorf("empty embedding received")
			}
		}

		// Validate result count
		if len(embeddings) != len(texts) {
			return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(embeddings))
		}

		c.logger.Debug().
			Int("count", len(embeddings)).
			Int("dimension", len(embeddings[0])).
			Dur("duration", time.Since(startTime)).
			Msg("Embeddings generated successfully")

		return embeddings, nil
	}

	return nil, fmt.Errorf("failed to generate embeddings after %d attempts: %w", maxRetries+1, lastErr)
}

// GetDimension returns the dimension of embeddings for this model
// text-embedding-004 produces 768-dimensional vectors
func (c *Client) GetDimension() int {
	switch c.model {
	case "text-embedding-004":
		return 768
	case "embedding-001":
		return 768
	default:
		return 768 // Default for Gemini embeddings
	}
}
