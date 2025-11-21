package summary

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/rs/zerolog"
	"github.com/telegram-llm-bot/internal/models"
	"google.golang.org/api/option"
)

// Generator handles daily summary generation using LLM
type Generator struct {
	apiKey      string
	config      *models.BotConfig
	logger      zerolog.Logger
	genaiClient *genai.Client
}

// NewGenerator creates a new summary generator
func NewGenerator(apiKey string, config *models.BotConfig, logger zerolog.Logger) *Generator {
	return &Generator{
		apiKey: apiKey,
		config: config,
		logger: logger.With().Str("component", "summary_generator").Logger(),
	}
}

// Close closes the generator and releases resources
func (g *Generator) Close() error {
	if g.genaiClient != nil {
		err := g.genaiClient.Close()
		g.genaiClient = nil
		if err != nil {
			g.logger.Error().Err(err).Msg("Failed to close Gemini client")
			return err
		}
		g.logger.Info().Msg("Summary generator client closed")
	}
	return nil
}

// getClient returns or creates a genai client
func (g *Generator) getClient(ctx context.Context) (*genai.Client, error) {
	if g.genaiClient != nil {
		return g.genaiClient, nil
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(g.apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	g.genaiClient = client
	g.logger.Info().Msg("Summary generator Gemini client created")
	return g.genaiClient, nil
}

// GenerateSummary generates a daily summary from messages
func (g *Generator) GenerateSummary(ctx context.Context, messages []models.ChatMessage, date string) (*models.SummaryResult, error) {
	if len(messages) == 0 {
		g.logger.Debug().Str("date", date).Msg("No messages to summarize")
		return &models.SummaryResult{
			Topics:       []string{},
			MessageCount: 0,
		}, nil
	}

	g.logger.Info().
		Str("date", date).
		Int("message_count", len(messages)).
		Msg("Starting summary generation")

	// Generate topics using LLM
	topics, err := g.generateTopics(ctx, messages, date)
	if err != nil {
		return nil, fmt.Errorf("failed to generate topics: %w", err)
	}

	result := &models.SummaryResult{
		Topics:       topics,
		MessageCount: len(messages),
	}

	g.logger.Info().
		Str("date", date).
		Int("topic_count", len(topics)).
		Msg("Summary generation completed")

	return result, nil
}

// generateTopics uses LLM to extract main discussion topics
func (g *Generator) generateTopics(ctx context.Context, messages []models.ChatMessage, date string) ([]string, error) {
	// Create timeout context for LLM request
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Get or create Gemini client
	client, err := g.getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get genai client: %w", err)
	}

	// Use Flash model for cost-effectiveness
	model := client.GenerativeModel(string(models.ModelFlash))

	// Configure generation parameters
	model.SetTemperature(0.7)
	model.SetTopP(0.95)
	model.SetTopK(40)
	model.SetMaxOutputTokens(2048)

	// Build the prompt
	prompt := g.buildSummaryPrompt(messages, date)

	g.logger.Debug().
		Str("date", date).
		Int("message_count", len(messages)).
		Int("prompt_length", len(prompt)).
		Msg("Sending request to LLM for topic extraction")

	// Generate content
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	// Extract text from response
	if resp == nil || len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("no response candidates from LLM")
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return nil, fmt.Errorf("no content parts in response")
	}

	// Extract text from all parts
	var responseText strings.Builder
	for _, part := range candidate.Content.Parts {
		if text, ok := part.(genai.Text); ok {
			responseText.WriteString(string(text))
		}
	}

	text := responseText.String()

	g.logger.Debug().
		Str("date", date).
		Int("response_length", len(text)).
		Msg("Received LLM response")

	// Parse topics from response
	topics := g.parseTopics(text)

	return topics, nil
}

// buildSummaryPrompt constructs the prompt for LLM
func (g *Generator) buildSummaryPrompt(messages []models.ChatMessage, date string) string {
	var sb strings.Builder

	sb.WriteString("Проанализируй следующие сообщения из группового чата за день ")
	sb.WriteString(date)
	sb.WriteString(" и выдели 5-7 основных тем обсуждения.\n\n")
	sb.WriteString("ВАЖНО:\n")
	sb.WriteString("1. Каждая тема должна быть краткой (максимум 5-7 слов)\n")
	sb.WriteString("2. Начинай каждую тему с подходящего эмодзи\n")
	sb.WriteString("3. Выводи каждую тему на отдельной строке\n")
	sb.WriteString("4. НЕ нумеруй темы, только эмодзи и текст\n")
	sb.WriteString("5. Сфокусируйся на самых обсуждаемых и важных темах\n")
	sb.WriteString("6. Если тем меньше 5, выведи только те что есть\n\n")

	sb.WriteString("Сообщения:\n\n")

	// Limit total prompt size to avoid token limits
	const maxMessagesInPrompt = 500
	messagesToUse := messages
	if len(messages) > maxMessagesInPrompt {
		// Take first 250 and last 250 messages to get context from beginning and end of day
		messagesToUse = append(messages[:250], messages[len(messages)-250:]...)
		sb.WriteString(fmt.Sprintf("[Показаны первые 250 и последние 250 сообщений из %d]\n\n", len(messages)))
	}

	for _, msg := range messagesToUse {
		// Format: [HH:MM] Username: Message text
		timestamp := msg.CreatedAt.Format("15:04")
		username := msg.Username
		if username == "" {
			username = msg.FirstName
		}
		if username == "" {
			username = fmt.Sprintf("User%d", msg.UserID)
		}

		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", timestamp, username, msg.MessageText))
	}

	sb.WriteString("\n\nТеперь выдели 5-7 основных тем в формате:\n")
	sb.WriteString("🎮 Тема 1\n")
	sb.WriteString("💻 Тема 2\n")
	sb.WriteString("и так далее...\n\n")
	sb.WriteString("Темы:")

	return sb.String()
}

// parseTopics extracts topic lines from LLM response
func (g *Generator) parseTopics(text string) []string {
	lines := strings.Split(text, "\n")
	topics := make([]string, 0, 7)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip lines that look like instructions or headers
		if strings.HasPrefix(strings.ToLower(line), "темы:") ||
			strings.HasPrefix(strings.ToLower(line), "основные темы") ||
			strings.HasPrefix(strings.ToLower(line), "вот") {
			continue
		}

		// Line should start with emoji or be a valid topic
		// Check if line has at least one emoji-like character (simple heuristic)
		hasEmoji := false
		for _, r := range line {
			if r > 0x1F000 { // Rough check for emoji range
				hasEmoji = true
				break
			}
		}

		// Accept lines with emoji or lines that look like topics
		if hasEmoji || (len(line) > 3 && len(line) < 100) {
			// Remove any numbering (1., 2., etc.)
			line = strings.TrimPrefix(line, "- ")
			for i := 1; i <= 10; i++ {
				line = strings.TrimPrefix(line, fmt.Sprintf("%d. ", i))
				line = strings.TrimPrefix(line, fmt.Sprintf("%d) ", i))
			}
			line = strings.TrimSpace(line)

			if line != "" && len(topics) < 7 {
				topics = append(topics, line)
			}
		}
	}

	g.logger.Debug().
		Int("parsed_topics", len(topics)).
		Strs("topics", topics).
		Msg("Parsed topics from LLM response")

	return topics
}
