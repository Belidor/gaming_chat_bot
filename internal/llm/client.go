package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/rs/zerolog"
	"github.com/telegram-llm-bot/internal/models"
	"google.golang.org/api/option"
)

// Client represents a Gemini LLM client
type Client struct {
	apiKey      string
	timeout     time.Duration
	config      *models.BotConfig
	logger      zerolog.Logger
	genaiClient *genai.Client
	mu          sync.Mutex
}

// NewClient creates a new Gemini LLM client
func NewClient(apiKey string, timeout int, config *models.BotConfig, logger zerolog.Logger) *Client {
	return &Client{
		apiKey:      apiKey,
		timeout:     time.Duration(timeout) * time.Second,
		config:      config,
		logger:      logger.With().Str("component", "llm").Logger(),
		genaiClient: nil, // Will be created on first use
	}
}

// getClient returns or creates a genai client (thread-safe)
func (c *Client) getClient(ctx context.Context) (*genai.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.genaiClient != nil {
		return c.genaiClient, nil
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(c.apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	c.genaiClient = client
	c.logger.Info().Msg("Gemini client created and cached")
	return c.genaiClient, nil
}

// Close closes the LLM client and releases resources
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.genaiClient != nil {
		err := c.genaiClient.Close()
		c.genaiClient = nil
		if err != nil {
			c.logger.Error().Err(err).Msg("Failed to close Gemini client")
			return err
		}
		c.logger.Info().Msg("Gemini client closed")
	}
	return nil
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
	// Get or create Gemini client (reused across requests)
	client, err := c.getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get genai client: %w", err)
	}

	// Get the model
	model := client.GenerativeModel(req.ModelType.String())

	// Configure generation with parameters from config
	model.SetTemperature(c.config.LLMTemperature)
	model.SetTopP(c.config.LLMTopP)
	model.SetTopK(c.config.LLMTopK)
	model.SetMaxOutputTokens(c.config.LLMMaxTokens)

	// Create prompt with length limitation
	prompt := fmt.Sprintf(SystemPromptTemplate, req.Text)

	c.logger.Debug().
		Int64("user_id", req.UserID).
		Str("model", req.ModelType.String()).
		Int("max_length", MaxResponseLength).
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
	if len([]rune(text)) > MaxResponseLength {
		runes := []rune(text)
		fallbackRunes := []rune(FallbackMessage)
		maxContentLength := MaxResponseLength - len(fallbackRunes)

		// Protection against too long fallback message
		if maxContentLength < 100 {
			// If fallback message is too long, truncate without it
			text = string(runes[:MaxResponseLength])
			c.logger.Warn().
				Int64("user_id", req.UserID).
				Str("model", req.ModelType.String()).
				Int("original_length", len(runes)).
				Int("truncated_length", MaxResponseLength).
				Msg("Response truncated without fallback (fallback too long)")
		} else {
			// Normal truncation with fallback
			text = string(runes[:maxContentLength]) + FallbackMessage
			c.logger.Warn().
				Int64("user_id", req.UserID).
				Str("model", req.ModelType.String()).
				Int("original_length", len(runes)).
				Int("truncated_length", len([]rune(text))).
				Msg("Response truncated to fit Telegram limit")
		}
	}

	c.logger.Info().
		Int64("user_id", req.UserID).
		Str("username", req.Username).
		Str("model", req.ModelType.String()).
		Int("response_length", len([]rune(text))).
		Msg("LLM response generated successfully")

	return &models.LLMResponse{
		Text:      text,
		ModelUsed: req.ModelType.String(),
		Length:    len([]rune(text)),
		Error:     nil,
	}, nil
}
