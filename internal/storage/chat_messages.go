package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/telegram-llm-bot/internal/models"
)

// SaveChatMessage saves a chat message to the database
func (c *Client) SaveChatMessage(ctx context.Context, msg *models.ChatMessage) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	return c.withRetry(ctx, "save_chat_message", func() error {
		// Prepare data for insert
		data := map[string]interface{}{
			"message_id":   msg.MessageID,
			"user_id":      msg.UserID,
			"username":     msg.Username,
			"first_name":   msg.FirstName,
			"chat_id":      msg.ChatID,
			"message_text": msg.MessageText,
			"indexed":      false,
			"created_at":   msg.CreatedAt,
		}

		// Insert message (ignore if already exists due to unique constraint)
		_, _, err := c.client.From("chat_messages").
			Insert(data, false, "", "", "").
			Execute()

		if err != nil {
			// Ignore duplicate key errors (message already saved)
			if contains(err.Error(), "duplicate") || contains(err.Error(), "unique") {
				c.logger.Debug().
					Int64("message_id", msg.MessageID).
					Int64("chat_id", msg.ChatID).
					Msg("Message already exists, skipping")
				return nil
			}
			return fmt.Errorf("failed to insert chat message: %w", err)
		}

		c.logger.Debug().
			Int64("message_id", msg.MessageID).
			Int64("user_id", msg.UserID).
			Str("username", msg.Username).
			Msg("Chat message saved successfully")

		return nil
	})
}

// GetUnindexedMessages retrieves messages that don't have embeddings yet
func (c *Client) GetUnindexedMessages(ctx context.Context, limit int) ([]*models.ChatMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var messages []*models.ChatMessage

	err := c.withRetry(ctx, "get_unindexed_messages", func() error {
		// Call PostgreSQL function
		data := c.client.Rpc("get_unindexed_messages", "", map[string]interface{}{
			"batch_size": limit,
		})

		if data == "" {
			return fmt.Errorf("failed to get unindexed messages: RPC returned empty")
		}

		// Parse response
		if err := json.Unmarshal([]byte(data), &messages); err != nil {
			return fmt.Errorf("failed to parse unindexed messages: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	c.logger.Debug().
		Int("count", len(messages)).
		Msg("Retrieved unindexed messages")

	return messages, nil
}

// UpdateMessageEmbedding updates a single message with its embedding
func (c *Client) UpdateMessageEmbedding(ctx context.Context, id int64, embedding []float32) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	return c.withRetry(ctx, "update_message_embedding", func() error {
		// Call PostgreSQL function
		data := c.client.Rpc("update_message_embedding", "", map[string]interface{}{
			"p_message_id": id,
			"p_embedding":  embedding,
		})

		if data == "" {
			return fmt.Errorf("failed to update message embedding: RPC returned empty")
		}

		c.logger.Debug().
			Int64("message_id", id).
			Msg("Message embedding updated")

		return nil
	})
}

// BatchUpdateEmbeddings updates multiple messages with embeddings in one operation
func (c *Client) BatchUpdateEmbeddings(ctx context.Context, ids []int64, embeddings [][]float32) (int, error) {
	if len(ids) != len(embeddings) {
		return 0, fmt.Errorf("ids and embeddings must have same length")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout*2) // Double timeout for batch operation
	defer cancel()

	var rowsUpdated int

	err := c.withRetry(ctx, "batch_update_embeddings", func() error {
		// Call PostgreSQL function
		data := c.client.Rpc("batch_update_embeddings", "", map[string]interface{}{
			"p_message_ids": ids,
			"p_embeddings":  embeddings,
		})

		if data == "" {
			return fmt.Errorf("failed to batch update embeddings: RPC returned empty")
		}

		// Parse result - function returns array with one row
		var results []struct {
			RowsUpdated int `json:"rows_updated"`
		}
		
		if err := json.Unmarshal([]byte(data), &results); err != nil {
			return fmt.Errorf("failed to parse batch update result: %w", err)
		}

		if len(results) > 0 {
			rowsUpdated = results[0].RowsUpdated
		} else {
			return fmt.Errorf("no result returned from batch_update_embeddings")
		}

		return nil
	})

	if err != nil {
		return 0, err
	}

	c.logger.Info().
		Int("count", rowsUpdated).
		Int("expected", len(ids)).
		Msg("Batch update embeddings completed")

	return rowsUpdated, nil
}

// SearchSimilarMessages searches for similar messages using vector similarity
func (c *Client) SearchSimilarMessages(
	ctx context.Context,
	queryEmbedding []float32,
	threshold float64,
	limit int,
	chatID int64,
) ([]*models.ChatMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var results []*models.ChatMessage

	err := c.withRetry(ctx, "search_similar_messages", func() error {
		// Prepare parameters
		params := map[string]interface{}{
			"query_embedding":      queryEmbedding,
			"similarity_threshold": threshold,
			"match_count":          limit,
		}

		// Add chat_id filter if specified
		if chatID != 0 {
			params["target_chat_id"] = chatID
		}

		// Call PostgreSQL function
		data := c.client.Rpc("search_similar_messages", "", params)

		if data == "" {
			// Empty result is OK - no similar messages found
			return nil
		}

		// Parse response
		if err := json.Unmarshal([]byte(data), &results); err != nil {
			return fmt.Errorf("failed to parse search results: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	c.logger.Debug().
		Int("count", len(results)).
		Float64("threshold", threshold).
		Msg("Similar messages found")

	return results, nil
}

// GetRAGStatistics retrieves RAG indexing statistics
func (c *Client) GetRAGStatistics(ctx context.Context) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var stats []map[string]interface{}

	err := c.withRetry(ctx, "get_rag_statistics", func() error {
		data, _, err := c.client.From("rag_statistics").
			Select("*", "exact", false).
			Execute()

		if err != nil {
			return fmt.Errorf("failed to get RAG statistics: %w", err)
		}

		if err := json.Unmarshal(data, &stats); err != nil {
			return fmt.Errorf("failed to parse RAG statistics: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if len(stats) == 0 {
		return map[string]interface{}{
			"total_messages":       0,
			"indexed_messages":     0,
			"unindexed_messages":   0,
			"indexed_percentage":   0.0,
			"oldest_message":       time.Time{},
			"newest_message":       time.Time{},
			"last_indexing":        time.Time{},
		}, nil
	}

	return stats[0], nil
}

// contains is a helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
