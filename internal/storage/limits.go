package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/telegram-llm-bot/internal/models"
)

// GetDailyLimit retrieves the daily limit record for a user on a specific date
func (c *Client) GetDailyLimit(ctx context.Context, userID int64, date string) (*models.DailyLimit, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Use RPC function to get current counts
	params := map[string]interface{}{
		"p_user_id": userID,
		"p_date":    date,
	}

	data := c.client.Rpc("get_daily_limit", "", params)
	if data == "" {
		c.logger.Debug().
			Int64("user_id", userID).
			Str("date", date).
			Msg("No existing limit found via RPC")

		// Return zero counts on empty response
		return &models.DailyLimit{
			UserID:             userID,
			Date:               date,
			ProRequestsCount:   0,
			FlashRequestsCount: 0,
			UpdatedAt:          time.Now().UTC(),
		}, nil
	}

	// Parse response
	var results []struct {
		ProCount   int `json:"pro_count"`
		FlashCount int `json:"flash_count"`
	}

	if err := json.Unmarshal([]byte(data), &results); err != nil {
		c.logger.Warn().
			Err(err).
			Msg("Failed to unmarshal RPC response, returning zero counts")

		return &models.DailyLimit{
			UserID:             userID,
			Date:               date,
			ProRequestsCount:   0,
			FlashRequestsCount: 0,
			UpdatedAt:          time.Now().UTC(),
		}, nil
	}

	proCount := 0
	flashCount := 0
	if len(results) > 0 {
		proCount = results[0].ProCount
		flashCount = results[0].FlashCount
	}

	c.logger.Debug().
		Int64("user_id", userID).
		Str("date", date).
		Int("pro_count", proCount).
		Int("flash_count", flashCount).
		Msg("Retrieved daily limit")

	return &models.DailyLimit{
		UserID:             userID,
		Date:               date,
		ProRequestsCount:   proCount,
		FlashRequestsCount: flashCount,
		UpdatedAt:          time.Now().UTC(),
	}, nil
}

// IncrementLimit increments the request count for a specific model
func (c *Client) IncrementLimit(ctx context.Context, userID int64, date string, modelType models.ModelType) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	operation := "increment_limit"
	err := c.withRetry(ctx, operation, func() error {
		// Determine model type string
		modelTypeStr := "flash"
		if modelType == models.ModelPro {
			modelTypeStr = "pro"
		}

		// Use RPC function for atomic increment
		params := map[string]interface{}{
			"p_user_id":    userID,
			"p_date":       date,
			"p_model_type": modelTypeStr,
		}

		result := c.client.Rpc("increment_daily_limit", "", params)
		if result == "" {
			return fmt.Errorf("failed to increment daily limit: RPC returned empty")
		}

		return nil
	})

	if err != nil {
		c.logger.Error().
			Err(err).
			Int64("user_id", userID).
			Str("date", date).
			Str("model", string(modelType)).
			Msg("Failed to increment limit")
		return err
	}

	c.logger.Debug().
		Int64("user_id", userID).
		Str("date", date).
		Str("model", string(modelType)).
		Msg("Limit incremented successfully")

	return nil
}

// isNotFoundError checks if error is a "not found" error
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// Check if error message contains common "not found" indicators
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "not found") ||
		strings.Contains(errMsg, "no rows") ||
		strings.Contains(errMsg, "pgrst116")
}
