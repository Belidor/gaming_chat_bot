package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	supa "github.com/supabase-community/supabase-go"
)

// Client represents a Supabase storage client
type Client struct {
	client  *supa.Client
	timeout time.Duration
	logger  zerolog.Logger
}

// NewClient creates a new Supabase client
func NewClient(supabaseURL, supabaseKey string, timeout int, logger zerolog.Logger) (*Client, error) {
	client, err := supa.NewClient(supabaseURL, supabaseKey, &supa.ClientOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create supabase client: %w", err)
	}

	return &Client{
		client:  client,
		timeout: time.Duration(timeout) * time.Second,
		logger:  logger.With().Str("component", "storage").Logger(),
	}, nil
}

// Ping checks if the connection to Supabase is working
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Simple query to check connection
	_, _, err := c.client.From("request_logs").
		Select("id", "exact", false).
		Limit(1, "").
		Execute()

	if err != nil {
		return fmt.Errorf("supabase ping failed: %w", err)
	}

	c.logger.Debug().Msg("Supabase connection successful")
	return nil
}

// withRetry executes a function with retry logic
func (c *Client) withRetry(ctx context.Context, operation string, fn func() error) error {
	maxRetries := 2
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(attempt) * 500 * time.Millisecond
			c.logger.Warn().
				Str("operation", operation).
				Int("attempt", attempt+1).
				Dur("backoff", backoff).
				Msg("Retrying operation")
			
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		c.logger.Error().
			Err(lastErr).
			Str("operation", operation).
			Int("attempt", attempt+1).
			Msg("Operation failed")
	}

	return fmt.Errorf("operation %s failed after %d attempts: %w", operation, maxRetries+1, lastErr)
}
