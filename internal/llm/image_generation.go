package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// Hugging Face Inference API endpoint (new router as of Nov 2025)
	// Old endpoint api-inference.huggingface.co is deprecated
	huggingFaceAPIURL = "https://router.huggingface.co/hf-inference/models"

	// Default model: FLUX.1-schnell is free and fast
	defaultImageModel = "black-forest-labs/FLUX.1-schnell"
)

// huggingFaceRequest represents the request body for Hugging Face API
type huggingFaceRequest struct {
	Inputs string `json:"inputs"`
}

// GenerateImage generates an image from a text prompt using Hugging Face Inference API
func (c *Client) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	startTime := time.Now()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	c.logger.Info().
		Str("prompt", prompt).
		Str("model", defaultImageModel).
		Msg("Starting image generation via Hugging Face")

	// Check if HF token is configured
	if c.config.HuggingFaceToken == "" {
		return nil, fmt.Errorf("hugging face token not configured")
	}

	// Prepare request
	reqBody := huggingFaceRequest{
		Inputs: prompt,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	apiURL := fmt.Sprintf("%s/%s", huggingFaceAPIURL, defaultImageModel)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+c.config.HuggingFaceToken)
	req.Header.Set("Content-Type", "application/json")

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.logger.Error().
			Err(err).
			Str("prompt", prompt).
			Msg("Failed to send request to Hugging Face API")
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		c.logger.Error().
			Int("status_code", resp.StatusCode).
			Str("response", string(imageData)).
			Str("prompt", prompt).
			Msg("Hugging Face API returned error")
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(imageData))
	}

	c.logger.Info().
		Int("size", len(imageData)).
		Dur("duration", time.Since(startTime)).
		Str("model", defaultImageModel).
		Msg("Image generated successfully via Hugging Face")

	return imageData, nil
}
