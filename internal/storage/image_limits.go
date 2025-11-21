package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/telegram-llm-bot/internal/models"
)

// GetUserImageGenerationsToday retrieves the number of image generations for a user today
func (c *Client) GetUserImageGenerationsToday(ctx context.Context, userID int64, date string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Use RPC function to get current image generation count for user
	params := map[string]interface{}{
		"p_user_id": userID,
		"p_date":    date,
	}

	data := c.client.Rpc("get_user_image_generations", "", params)
	if data == "" {
		c.logger.Debug().
			Int64("user_id", userID).
			Str("date", date).
			Msg("No existing image generations found for user")
		return 0, nil
	}

	// Parse response
	var results []struct {
		ImageGenerationsUsed int `json:"image_generations_used"`
	}

	if err := json.Unmarshal([]byte(data), &results); err != nil {
		c.logger.Warn().
			Err(err).
			Msg("Failed to unmarshal image generations RPC response, returning zero")
		return 0, nil
	}

	count := 0
	if len(results) > 0 {
		count = results[0].ImageGenerationsUsed
	}

	c.logger.Debug().
		Int64("user_id", userID).
		Str("date", date).
		Int("count", count).
		Msg("Retrieved user image generations count")

	return count, nil
}

// GetChatImageGenerationsToday retrieves the number of image generations for a chat today
func (c *Client) GetChatImageGenerationsToday(ctx context.Context, chatID int64, date string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Use RPC function to get current image generation count for chat
	params := map[string]interface{}{
		"p_chat_id": chatID,
		"p_date":    date,
	}

	data := c.client.Rpc("get_chat_image_generations", "", params)
	if data == "" {
		c.logger.Debug().
			Int64("chat_id", chatID).
			Str("date", date).
			Msg("No existing image generations found for chat")
		return 0, nil
	}

	// Parse response
	var results []struct {
		ImageGenerationsCount int `json:"image_generations_count"`
	}

	if err := json.Unmarshal([]byte(data), &results); err != nil {
		c.logger.Warn().
			Err(err).
			Msg("Failed to unmarshal chat image generations RPC response, returning zero")
		return 0, nil
	}

	count := 0
	if len(results) > 0 {
		count = results[0].ImageGenerationsCount
	}

	c.logger.Debug().
		Int64("chat_id", chatID).
		Str("date", date).
		Int("count", count).
		Msg("Retrieved chat image generations count")

	return count, nil
}

// CheckImageGenerationLimit checks if the user and chat have not exceeded their daily limits
func (c *Client) CheckImageGenerationLimit(ctx context.Context, userID, chatID int64, date string, config *models.BotConfig) (allowed bool, remaining int, err error) {
	// Check user limit
	userCount, err := c.GetUserImageGenerationsToday(ctx, userID, date)
	if err != nil {
		return false, 0, fmt.Errorf("failed to get user image generations: %w", err)
	}

	if userCount >= config.ImageGenerationDailyLimitPerUser {
		c.logger.Info().
			Int64("user_id", userID).
			Int("count", userCount).
			Int("limit", config.ImageGenerationDailyLimitPerUser).
			Msg("User image generation limit exceeded")
		return false, 0, nil
	}

	// Check chat limit
	chatCount, err := c.GetChatImageGenerationsToday(ctx, chatID, date)
	if err != nil {
		return false, 0, fmt.Errorf("failed to get chat image generations: %w", err)
	}

	if chatCount >= config.ImageGenerationDailyLimitPerChat {
		c.logger.Info().
			Int64("chat_id", chatID).
			Int("count", chatCount).
			Int("limit", config.ImageGenerationDailyLimitPerChat).
			Msg("Chat image generation limit exceeded")
		return false, 0, nil
	}

	// Calculate remaining (minimum of user and chat remaining)
	userRemaining := config.ImageGenerationDailyLimitPerUser - userCount
	chatRemaining := config.ImageGenerationDailyLimitPerChat - chatCount
	remaining = userRemaining
	if chatRemaining < userRemaining {
		remaining = chatRemaining
	}

	c.logger.Debug().
		Int64("user_id", userID).
		Int64("chat_id", chatID).
		Int("user_count", userCount).
		Int("chat_count", chatCount).
		Int("remaining", remaining).
		Msg("Image generation limit check passed")

	return true, remaining, nil
}

// RecordImageGeneration records an image generation for both user and chat statistics
func (c *Client) RecordImageGeneration(ctx context.Context, userID, chatID int64, date string) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	operation := "record_image_generation"
	err := c.withRetry(ctx, operation, func() error {
		// Use RPC function to atomically increment both counters
		params := map[string]interface{}{
			"p_user_id": userID,
			"p_chat_id": chatID,
			"p_date":    date,
		}

		result := c.client.Rpc("record_image_generation", "", params)
		if result == "" {
			return fmt.Errorf("failed to record image generation: RPC returned empty")
		}

		return nil
	})

	if err != nil {
		c.logger.Error().
			Err(err).
			Int64("user_id", userID).
			Int64("chat_id", chatID).
			Str("date", date).
			Msg("Failed to record image generation")
		return err
	}

	c.logger.Debug().
		Int64("user_id", userID).
		Int64("chat_id", chatID).
		Str("date", date).
		Msg("Image generation recorded successfully")

	return nil
}
