package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/telegram-llm-bot/internal/models"
)

// SaveDailySummary stores a generated daily summary in the database
// Uses upsert to allow overwriting existing summaries (for force regeneration)
func (c *Client) SaveDailySummary(ctx context.Context, summary *models.DailySummary) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Set created_at if not set
	if summary.CreatedAt.IsZero() {
		summary.CreatedAt = time.Now().UTC()
	}

	operation := "save_daily_summary"
	err := c.withRetry(ctx, operation, func() error {
		data := map[string]interface{}{
			"chat_id":              summary.ChatID,
			"date":                 summary.Date,
			"summary_text":         summary.SummaryText,
			"most_active_user_id":  summary.MostActiveUserID,
			"most_active_username": summary.MostActiveUsername,
			"message_count":        summary.MessageCount,
			"created_at":           summary.CreatedAt,
		}

		_, _, err := c.client.From("daily_summaries").
			Insert(data, true, "chat_id,date", "", "").
			Execute()

		if err != nil {
			return fmt.Errorf("failed to upsert daily summary: %w", err)
		}

		return nil
	})

	if err != nil {
		c.logger.Error().
			Err(err).
			Int64("chat_id", summary.ChatID).
			Str("date", summary.Date).
			Msg("Failed to save daily summary")
		return err
	}

	c.logger.Info().
		Int64("chat_id", summary.ChatID).
		Str("date", summary.Date).
		Int("message_count", summary.MessageCount).
		Str("most_active_user", summary.MostActiveUsername).
		Msg("Daily summary saved successfully")

	return nil
}

// SummaryExistsForDate checks if a summary already exists for a specific date
func (c *Client) SummaryExistsForDate(ctx context.Context, chatID int64, date string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var summaries []models.DailySummary
	operation := "check_summary_exists"

	err := c.withRetry(ctx, operation, func() error {
		data, _, err := c.client.From("daily_summaries").
			Select("id", "exact", false).
			Eq("chat_id", fmt.Sprintf("%d", chatID)).
			Eq("date", date).
			Limit(1, "").
			Execute()

		if err != nil {
			return fmt.Errorf("failed to check summary existence: %w", err)
		}

		if err := json.Unmarshal(data, &summaries); err != nil {
			return fmt.Errorf("failed to unmarshal summaries: %w", err)
		}

		return nil
	})

	if err != nil {
		c.logger.Error().
			Err(err).
			Int64("chat_id", chatID).
			Str("date", date).
			Msg("Failed to check if summary exists")
		return false, err
	}

	exists := len(summaries) > 0

	c.logger.Debug().
		Int64("chat_id", chatID).
		Str("date", date).
		Bool("exists", exists).
		Msg("Checked summary existence")

	return exists, nil
}

// GetDailySummary retrieves a daily summary for a specific date
func (c *Client) GetDailySummary(ctx context.Context, chatID int64, date string) (*models.DailySummary, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var summaries []models.DailySummary
	operation := "get_daily_summary"

	err := c.withRetry(ctx, operation, func() error {
		data, _, err := c.client.From("daily_summaries").
			Select("*", "exact", false).
			Eq("chat_id", fmt.Sprintf("%d", chatID)).
			Eq("date", date).
			Limit(1, "").
			Execute()

		if err != nil {
			return fmt.Errorf("failed to fetch daily summary: %w", err)
		}

		if err := json.Unmarshal(data, &summaries); err != nil {
			return fmt.Errorf("failed to unmarshal summary: %w", err)
		}

		return nil
	})

	if err != nil {
		c.logger.Error().
			Err(err).
			Int64("chat_id", chatID).
			Str("date", date).
			Msg("Failed to get daily summary")
		return nil, err
	}

	if len(summaries) == 0 {
		return nil, nil
	}

	c.logger.Debug().
		Int64("chat_id", chatID).
		Str("date", date).
		Msg("Retrieved daily summary")

	return &summaries[0], nil
}
