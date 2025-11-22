package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/telegram-llm-bot/internal/models"
)

// GetMessagesForDate retrieves all messages for a specific date in Moscow timezone
func (c *Client) GetMessagesForDate(ctx context.Context, chatID int64, date string) ([]models.ChatMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return nil, fmt.Errorf("failed to load timezone: %w", err)
	}

	startTime, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return nil, fmt.Errorf("failed to parse date: %w", err)
	}
	endTime := startTime.AddDate(0, 0, 1)

	startUTC := startTime.UTC()
	endUTC := endTime.UTC()

	var messages []models.ChatMessage
	operation := "get_messages_for_date"

	err = c.withRetry(ctx, operation, func() error {
		data, _, err := c.client.From("chat_messages").
			Select("id,message_id,user_id,username,first_name,chat_id,message_text,indexed,created_at,indexed_at", "exact", false).
			Eq("chat_id", fmt.Sprintf("%d", chatID)).
			Gte("created_at", startUTC.Format(time.RFC3339)).
			Lt("created_at", endUTC.Format(time.RFC3339)).
			Order("created_at", nil).
			Execute()

		if err != nil {
			return fmt.Errorf("failed to fetch messages: %w", err)
		}

		if err := json.Unmarshal(data, &messages); err != nil {
			return fmt.Errorf("failed to unmarshal messages: %w", err)
		}

		return nil
	})

	if err != nil {
		c.logger.Error().
			Err(err).
			Int64("chat_id", chatID).
			Str("date", date).
			Msg("Failed to get messages for date")
		return nil, err
	}

	filtered := make([]models.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		msgDate := msg.CreatedAt.In(loc).Format("2006-01-02")
		if msgDate == date {
			filtered = append(filtered, msg)
		}
	}

	c.logger.Debug().
		Int64("chat_id", chatID).
		Str("date", date).
		Int("retrieved_count", len(messages)).
		Int("filtered_count", len(filtered)).
		Msg("Filtered messages for date in Moscow timezone")

	return filtered, nil
}

// GetUserMessageCounts retrieves message counts per user for a specific date
func (c *Client) GetUserMessageCounts(ctx context.Context, chatID int64, date string) ([]models.UserMessageCount, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Note: Supabase Go client doesn't support GROUP BY directly
	// We'll fetch all messages and count them in Go
	messages, err := c.GetMessagesForDate(ctx, chatID, date)
	if err != nil {
		return nil, err
	}

	// Count messages per user
	userCounts := make(map[int64]*models.UserMessageCount)
	for _, msg := range messages {
		if count, exists := userCounts[msg.UserID]; exists {
			count.MessageCount++
		} else {
			userCounts[msg.UserID] = &models.UserMessageCount{
				UserID:       msg.UserID,
				Username:     msg.Username,
				FirstName:    msg.FirstName,
				MessageCount: 1,
			}
		}
	}

	// Convert map to slice
	result := make([]models.UserMessageCount, 0, len(userCounts))
	for _, count := range userCounts {
		result = append(result, *count)
	}

	c.logger.Debug().
		Int64("chat_id", chatID).
		Str("date", date).
		Int("user_count", len(result)).
		Msg("Calculated user message counts")

	return result, nil
}

// GetMostActiveUser finds the user with the most messages for a specific date
func (c *Client) GetMostActiveUser(ctx context.Context, chatID int64, date string) (*models.UserMessageCount, error) {
	counts, err := c.GetUserMessageCounts(ctx, chatID, date)
	if err != nil {
		return nil, err
	}

	if len(counts) == 0 {
		return nil, nil
	}

	// Find user with most messages
	mostActive := &counts[0]
	for i := 1; i < len(counts); i++ {
		if counts[i].MessageCount > mostActive.MessageCount {
			mostActive = &counts[i]
		}
	}

	c.logger.Debug().
		Int64("chat_id", chatID).
		Str("date", date).
		Int64("user_id", mostActive.UserID).
		Str("username", mostActive.Username).
		Int("message_count", mostActive.MessageCount).
		Msg("Found most active user")

	return mostActive, nil
}
