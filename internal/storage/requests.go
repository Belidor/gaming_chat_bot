package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/telegram-llm-bot/internal/models"
)

// LogRequest logs a request to the database
func (c *Client) LogRequest(ctx context.Context, log *models.RequestLog) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Set created_at if not set
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}

	operation := "log_request"
	err := c.withRetry(ctx, operation, func() error {
		data := map[string]interface{}{
			"user_id":           log.UserID,
			"username":          log.Username,
			"first_name":        log.FirstName,
			"chat_id":           log.ChatID,
			"request_text":      log.RequestText,
			"response_text":     log.ResponseText,
			"model_used":        log.ModelUsed,
			"response_length":   log.ResponseLength,
			"execution_time_ms": log.ExecutionTimeMs,
			"error_message":     log.ErrorMessage,
			"created_at":        log.CreatedAt,
		}

		_, _, err := c.client.From("request_logs").
			Insert(data, false, "", "", "").
			Execute()

		if err != nil {
			return fmt.Errorf("failed to insert request log: %w", err)
		}

		return nil
	})

	if err != nil {
		c.logger.Error().
			Err(err).
			Int64("user_id", log.UserID).
			Str("model", log.ModelUsed).
			Msg("Failed to log request")
		return err
	}

	c.logger.Debug().
		Int64("user_id", log.UserID).
		Str("username", log.Username).
		Str("model", log.ModelUsed).
		Int("response_len", log.ResponseLength).
		Int("exec_time_ms", log.ExecutionTimeMs).
		Msg("Request logged successfully")

	return nil
}

// GetUserTotalRequests returns total number of requests made by a user
func (c *Client) GetUserTotalRequests(ctx context.Context, userID int64) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	_, count, err := c.client.From("request_logs").
		Select("id", "exact", false).
		Eq("user_id", fmt.Sprintf("%d", userID)).
		Execute()

	if err != nil {
		c.logger.Error().
			Err(err).
			Int64("user_id", userID).
			Msg("Failed to get user total requests")
		return 0, fmt.Errorf("failed to get user total requests: %w", err)
	}

	return count, nil
}
