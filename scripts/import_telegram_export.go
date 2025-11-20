package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/telegram-llm-bot/internal/config"
	"github.com/telegram-llm-bot/internal/embeddings"
	"github.com/telegram-llm-bot/internal/models"
	"github.com/telegram-llm-bot/internal/storage"
)

// TelegramExport represents Telegram Desktop JSON export format
type TelegramExport struct {
	Name     string                   `json:"name"`
	Type     string                   `json:"type"`
	ID       int64                    `json:"id"`
	Messages []TelegramExportMessage `json:"messages"`
}

// TelegramExportMessage represents a message in Telegram export
type TelegramExportMessage struct {
	ID           int64  `json:"id"`
	Type         string `json:"type"`
	Date         string `json:"date"`
	DateUnixtime string `json:"date_unixtime"`
	From         string `json:"from"`
	FromID       string `json:"from_id"`
	Text         interface{} `json:"text"` // Can be string or array
	TextEntities []interface{} `json:"text_entities,omitempty"`
}

func main() {
	// Parse command-line flags
	exportFile := flag.String("file", "", "Path to Telegram export JSON file (required)")
	dryRun := flag.Bool("dry-run", false, "Dry run mode (don't save to database)")
	flag.Parse()

	if *exportFile == "" {
		fmt.Println("Usage: go run scripts/import_telegram_export.go -file=result.json")
		fmt.Println("       go run scripts/import_telegram_export.go -file=result.json -dry-run")
		os.Exit(1)
	}

	// Load configuration
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Setup logger
	logger := zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}).With().Timestamp().Logger()

	logger.Info().
		Str("file", *exportFile).
		Bool("dry_run", *dryRun).
		Msg("Starting Telegram history import")

	// Read export file
	file, err := os.Open(*exportFile)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to open export file")
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to read export file")
	}

	// Parse JSON
	var export TelegramExport
	if err := json.Unmarshal(data, &export); err != nil {
		logger.Fatal().Err(err).Msg("Failed to parse export JSON")
	}

	// Normalize chat_id to Telegram Bot API format
	// Telegram Desktop exports use positive IDs (e.g. 1750074031)
	// Telegram Bot API uses negative IDs for supergroups (e.g. -1001750074031)
	chatID := normalizeChatID(export.ID)

	logger.Info().
		Str("chat_name", export.Name).
		Int64("export_chat_id", export.ID).
		Int64("normalized_chat_id", chatID).
		Int("total_messages", len(export.Messages)).
		Msg("Export file parsed successfully")

	if *dryRun {
		logger.Info().Msg("DRY RUN MODE - No changes will be made to database")
		// Print sample messages
		sampleCount := 5
		if len(export.Messages) < sampleCount {
			sampleCount = len(export.Messages)
		}
		for i := 0; i < sampleCount; i++ {
			msg := export.Messages[i]
			text := extractText(msg.Text)
			logger.Info().
				Int64("message_id", msg.ID).
				Str("from", msg.From).
				Str("text", truncate(text, 100)).
				Msg("Sample message")
		}
		return
	}

	// Initialize storage
	storageClient, err := storage.NewClient(
		cfg.SupabaseURL,
		cfg.SupabaseKey,
		cfg.SupabaseTimeout,
		logger,
	)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create storage client")
	}

	// Initialize embeddings client
	embeddingsClient := embeddings.NewClient(
		cfg.GeminiAPIKey,
		cfg.RAG.EmbeddingsModel,
		cfg.RAG.EmbeddingsBatchSize,
		time.Duration(cfg.GeminiTimeout)*time.Second,
		logger,
	)
	defer embeddingsClient.Close()

	ctx := context.Background()
	startTime := time.Now()

	// Filter text messages only
	var textMessages []TelegramExportMessage
	for _, msg := range export.Messages {
		if msg.Type == "message" && msg.Text != nil {
			text := extractText(msg.Text)
			if text != "" && len(text) > 0 {
				textMessages = append(textMessages, msg)
			}
		}
	}

	logger.Info().
		Int("text_messages", len(textMessages)).
		Msg("Filtered text messages")

	// Save messages to database
	saved := 0
	for _, msg := range textMessages {
		text := extractText(msg.Text)
		
		// Parse timestamp
		timestamp, err := parseTimestamp(msg.DateUnixtime)
		if err != nil {
			logger.Warn().
				Err(err).
				Int64("message_id", msg.ID).
				Msg("Failed to parse timestamp, using now")
			timestamp = time.Now()
		}

		// Parse user_id from from_id field
		userID := parseUserID(msg.FromID)

		// Create chat message
		chatMsg := &models.ChatMessage{
			MessageID:   msg.ID,
			UserID:      userID,
			Username:    msg.From,
			FirstName:   msg.From,
			ChatID:      chatID, // Use normalized chat_id
			MessageText: text,
			Indexed:     false,
			CreatedAt:   timestamp,
		}

		// Save to database
		if err := storageClient.SaveChatMessage(ctx, chatMsg); err != nil {
			logger.Error().
				Err(err).
				Int64("message_id", msg.ID).
				Msg("Failed to save message")
			continue
		}

		saved++
		if saved%100 == 0 {
			logger.Info().Int("saved", saved).Msg("Progress...")
		}
	}

	logger.Info().
		Int("total", len(textMessages)).
		Int("saved", saved).
		Msg("Messages saved to database")

	// Now index them
	logger.Info().Msg("Starting embedding generation...")

	batchSize := cfg.RAG.EmbeddingsBatchSize
	indexed := 0

	for i := 0; i < saved; i += batchSize {
		// Get batch of unindexed messages
		messages, err := storageClient.GetUnindexedMessages(ctx, batchSize)
		if err != nil || len(messages) == 0 {
			break
		}

		// Extract texts
		texts := make([]string, len(messages))
		ids := make([]int64, len(messages))
		for j, msg := range messages {
			texts[j] = msg.MessageText
			ids[j] = msg.ID
		}

		// Generate embeddings
		logger.Info().
			Int("batch_start", i).
			Int("batch_size", len(messages)).
			Msg("Generating embeddings for batch")

		embeds, err := embeddingsClient.GenerateEmbeddingsBatch(ctx, texts)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to generate embeddings for batch")
			continue
		}

		// Update database
		updated, err := storageClient.BatchUpdateEmbeddings(ctx, ids, embeds)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to update embeddings")
			continue
		}

		indexed += updated
		logger.Info().
			Int("batch_indexed", updated).
			Int("total_indexed", indexed).
			Msg("Batch indexed")
	}

	duration := time.Since(startTime)

	logger.Info().
		Int("total_saved", saved).
		Int("total_indexed", indexed).
		Dur("duration", duration).
		Msg("History migration completed successfully")

	fmt.Printf("\n✅ Migration Complete!\n")
	fmt.Printf("   Messages saved: %d\n", saved)
	fmt.Printf("   Messages indexed: %d\n", indexed)
	fmt.Printf("   Duration: %.1f seconds\n", duration.Seconds())
}

// extractText extracts text from message.text field (can be string or array)
func extractText(text interface{}) string {
	switch v := text.(type) {
	case string:
		return v
	case []interface{}:
		// Text with entities - concatenate all text parts
		var result string
		for _, part := range v {
			if str, ok := part.(string); ok {
				result += str
			} else if m, ok := part.(map[string]interface{}); ok {
				if txt, ok := m["text"].(string); ok {
					result += txt
				}
			}
		}
		return result
	default:
		return ""
	}
}

// parseTimestamp parses Unix timestamp string
func parseTimestamp(unixtime string) (time.Time, error) {
	if unixtime == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	
	var timestamp int64
	_, err := fmt.Sscanf(unixtime, "%d", &timestamp)
	if err != nil {
		return time.Time{}, err
	}
	
	return time.Unix(timestamp, 0), nil
}

// truncate truncates string to max length
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// normalizeChatID normalizes chat_id to Telegram Bot API format
// Telegram Desktop JSON exports use positive supergroup IDs (e.g. 1750074031)
// Telegram Bot API uses format: -100{group_id} (e.g. -1001750074031)
// This function converts positive supergroup IDs to Bot API format
func normalizeChatID(chatID int64) int64 {
	// If already negative, assume it's already in correct format
	if chatID < 0 {
		return chatID
	}

	// If positive and looks like a supergroup ID (large number > 1 billion)
	// Convert to Bot API format: -100 prefix
	if chatID > 1000000000 {
		return -1000000000000 - chatID
	}

	// For private chats or small IDs, keep as is
	return chatID
}

// parseUserID extracts user_id from Telegram export from_id field
// Telegram Desktop exports use format: "user123456789" or "channel123456789"
// We extract the numeric part
func parseUserID(fromID string) int64 {
	if fromID == "" {
		return 0
	}

	// Remove "user" or "channel" prefix
	var userID int64
	
	// Try formats: "user123456789", "channel123456789", or just "123456789"
	if _, err := fmt.Sscanf(fromID, "user%d", &userID); err == nil {
		return userID
	}
	if _, err := fmt.Sscanf(fromID, "channel%d", &userID); err == nil {
		return -userID // Channels use negative IDs
	}
	if _, err := fmt.Sscanf(fromID, "%d", &userID); err == nil {
		return userID
	}

	// If parsing failed, return 0
	return 0
}
