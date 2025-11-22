package rag

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog"
	"github.com/telegram-llm-bot/internal/embeddings"
	"github.com/telegram-llm-bot/internal/models"
	"github.com/telegram-llm-bot/internal/storage"
)

// Searcher performs RAG search over chat history
type Searcher struct {
	storage          *storage.Client
	embeddingsClient *embeddings.Client
	config           models.RAGConfig
	logger           zerolog.Logger
}

// NewSearcher creates a new RAG searcher
func NewSearcher(
	storage *storage.Client,
	embeddingsClient *embeddings.Client,
	config models.RAGConfig,
	logger zerolog.Logger,
) *Searcher {
	return &Searcher{
		storage:          storage,
		embeddingsClient: embeddingsClient,
		config:           config,
		logger:           logger.With().Str("component", "rag").Logger(),
	}
}

// Search performs RAG search for relevant messages
func (s *Searcher) Search(ctx context.Context, query string, chatID int64) (*models.RAGResult, error) {
	if !s.config.Enabled {
		s.logger.Debug().Msg("RAG is disabled")
		return &models.RAGResult{
			Context:   "",
			Messages:  []*models.ChatMessage{},
			QueryUsed: query,
			Count:     0,
		}, nil
	}

	startTime := time.Now()

	// 1. Generate embedding for query
	s.logger.Debug().
		Str("query", truncate(query, 50)).
		Msg("Generating query embedding")

	queryEmbedding, err := s.embeddingsClient.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// 2. Search for similar messages
	s.logger.Debug().
		Float64("threshold", s.config.SimilarityThreshold).
		Int("top_k", s.config.TopK).
		Msg("Searching for similar messages")

	similarMessages, err := s.storage.SearchSimilarMessages(
		ctx,
		queryEmbedding,
		s.config.SimilarityThreshold,
		s.config.TopK,
		chatID,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to search similar messages: %w", err)
	}

	// 3. Format context
	context := s.FormatContext(similarMessages)

	// 4. Create result
	result := &models.RAGResult{
		Context:   context,
		Messages:  similarMessages,
		QueryUsed: query,
		Count:     len(similarMessages),
	}

	s.logger.Info().
		Int("results_count", len(similarMessages)).
		Dur("duration", time.Since(startTime)).
		Msg("RAG search completed")

	return result, nil
}

// FormatContext formats search results into a context string for LLM
func (s *Searcher) FormatContext(messages []*models.ChatMessage) string {
	if len(messages) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("РЕЛЕВАНТНАЯ ИНФОРМАЦИЯ ИЗ ИСТОРИИ ЧАТА:\n\n")

	totalLength := 0
	maxLength := s.config.MaxContextLength

	for i, msg := range messages {
		// Format: "1. Вася (2 дня назад, similarity: 0.89): сообщение"
		author := formatAuthor(msg)
		timeAgo := formatTimeAgo(msg.CreatedAt)
		similarity := fmt.Sprintf("%.2f", msg.Similarity)

		entry := fmt.Sprintf("%d. %s (%s, релевантность: %s): \"%s\"\n",
			i+1, author, timeAgo, similarity, msg.MessageText)

		entryRunes := utf8.RuneCountInString(entry)
		if totalLength+entryRunes > maxLength {
			builder.WriteString(fmt.Sprintf("\n[... еще %d релевантных сообщений не показаны из-за ограничения длины]\n", len(messages)-i))
			break
		}

		builder.WriteString(entry)
		totalLength += entryRunes
	}

	builder.WriteString("\n")
	return builder.String()
}

// formatAuthor formats message author name
func formatAuthor(msg *models.ChatMessage) string {
	if msg.FirstName != "" {
		return msg.FirstName
	}
	if msg.Username != "" {
		return "@" + msg.Username
	}
	return fmt.Sprintf("User_%d", msg.UserID)
}

// formatTimeAgo formats time ago in Russian
func formatTimeAgo(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "только что"
	case diff < time.Hour:
		minutes := int(diff.Minutes())
		return fmt.Sprintf("%d %s назад", minutes, pluralizeRu(minutes, "минута", "минуты", "минут"))
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		return fmt.Sprintf("%d %s назад", hours, pluralizeRu(hours, "час", "часа", "часов"))
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%d %s назад", days, pluralizeRu(days, "день", "дня", "дней"))
	case diff < 30*24*time.Hour:
		weeks := int(diff.Hours() / 24 / 7)
		return fmt.Sprintf("%d %s назад", weeks, pluralizeRu(weeks, "неделя", "недели", "недель"))
	case diff < 365*24*time.Hour:
		months := int(diff.Hours() / 24 / 30)
		return fmt.Sprintf("%d %s назад", months, pluralizeRu(months, "месяц", "месяца", "месяцев"))
	default:
		years := int(diff.Hours() / 24 / 365)
		return fmt.Sprintf("%d %s назад", years, pluralizeRu(years, "год", "года", "лет"))
	}
}

// pluralizeRu returns correct Russian plural form
func pluralizeRu(n int, form1, form2, form5 string) string {
	n = abs(n) % 100
	if n >= 11 && n <= 19 {
		return form5
	}
	n = n % 10
	if n == 1 {
		return form1
	}
	if n >= 2 && n <= 4 {
		return form2
	}
	return form5
}

// abs returns absolute value
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// truncate truncates string to maxLen characters
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	if maxLen > len(runes) {
		maxLen = len(runes)
	}
	return string(runes[:maxLen]) + "..."
}
