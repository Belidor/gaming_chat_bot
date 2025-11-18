package llm

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

// Client represents a Gemini LLM client
type Client struct {
	apiKey  string
	timeout time.Duration
	logger  zerolog.Logger
}

// NewClient creates a new Gemini LLM client
func NewClient(apiKey string, timeout int, logger zerolog.Logger) *Client {
	return &Client{
		apiKey:  apiKey,
		timeout: time.Duration(timeout) * time.Second,
		logger:  logger.With().Str("component", "llm").Logger(),
	}
}

// GenerateResponse generates a response from LLM
func (c *Client) GenerateResponse(ctx context.Context, req *models.LLMRequest) *models.LLMResponse {
	startTime := time.Now()
	
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Try to generate response with retry
	response := c.generateWithRetry(ctx, req)
	
	// Calculate execution time
	response.ExecutionTimeMs = int(time.Since(startTime).Milliseconds())

	return response
}

// generateWithRetry attempts to generate response with retry logic
func (c *Client) generateWithRetry(ctx context.Context, req *models.LLMRequest) *models.LLMResponse {
	maxRetries := 3
	var lastError error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			c.logger.Warn().
				Int("attempt", attempt+1).
				Dur("backoff", backoff).
				Int64("user_id", req.UserID).
				Msg("Retrying LLM request")

			select {
			case <-ctx.Done():
				return &models.LLMResponse{
					Text:      "",
					ModelUsed: req.ModelType.String(),
					Error:     ctx.Err(),
				}
			case <-time.After(backoff):
			}
		}

		// Attempt to generate response
		response, err := c.generate(ctx, req)
		if err == nil {
			return response
		}

		lastError = err
		c.logger.Error().
			Err(err).
			Int("attempt", attempt+1).
			Int64("user_id", req.UserID).
			Str("model", req.ModelType.String()).
			Msg("LLM request failed")
	}

	// All retries failed
	return &models.LLMResponse{
		Text:      "",
		ModelUsed: req.ModelType.String(),
		Error:     fmt.Errorf("failed after %d attempts: %w", maxRetries+1, lastError),
	}
}

// generate makes actual API call to Gemini
func (c *Client) generate(ctx context.Context, req *models.LLMRequest) (*models.LLMResponse, error) {
	// Create Gemini client
	client, err := genai.NewClient(ctx, option.WithAPIKey(c.apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}
	defer client.Close()

	// Get the model
	model := client.GenerativeModel(req.ModelType.String())
	
	// Configure generation
	model.SetTemperature(0.7)
	model.SetTopP(0.95)
	model.SetTopK(40)
	model.SetMaxOutputTokens(8192)

	// Create prompt with length limitation
	prompt := fmt.Sprintf(SystemPromptTemplate, req.MaxLength, req.Text)

	c.logger.Debug().
		Int64("user_id", req.UserID).
		Str("model", req.ModelType.String()).
		Int("max_length", req.MaxLength).
		Msg("Sending request to LLM")

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
	
	// Check if response exceeds max length
	if len(text) > req.MaxLength {
		c.logger.Warn().
			Int64("user_id", req.UserID).
			Str("model", req.ModelType.String()).
			Int("actual_length", len(text)).
			Int("max_length", req.MaxLength).
			Msg("Response exceeds max length, truncating")

		// Truncate and add fallback message
		truncateAt := req.MaxLength - len(FallbackMessage)
		if truncateAt < 0 {
			truncateAt = req.MaxLength
		}
		text = text[:truncateAt] + FallbackMessage
	}

	c.logger.Info().
		Int64("user_id", req.UserID).
		Str("username", req.Username).
		Str("model", req.ModelType.String()).
		Int("response_length", len(text)).
		Msg("LLM response generated successfully")

	return &models.LLMResponse{
		Text:      text,
		ModelUsed: req.ModelType.String(),
		Length:    len(text),
		Error:     nil,
	}, nil
}
