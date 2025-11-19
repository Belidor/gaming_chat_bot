package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/telegram-llm-bot/internal/models"
)

// Load loads configuration from environment variables
// It first attempts to load from .env file, then reads environment variables
func Load() (*models.BotConfig, error) {
	// Try to load .env file (optional, ignore error if not found)
	_ = godotenv.Load()

	config := &models.BotConfig{
		// Telegram settings
		TelegramToken:    getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramUsername: getEnv("TELEGRAM_BOT_USERNAME", ""),
		AllowedChatIDs:   getEnvInt64List("TELEGRAM_ALLOWED_CHAT_IDS", nil),

		// Gemini API settings
		GeminiAPIKey:  getEnv("GEMINI_API_KEY", ""),
		GeminiTimeout: getEnvInt("GEMINI_TIMEOUT", 30),

		// Supabase settings
		SupabaseURL:     getEnv("SUPABASE_URL", ""),
		SupabaseKey:     getEnv("SUPABASE_KEY", ""),
		SupabaseTimeout: getEnvInt("SUPABASE_TIMEOUT", 10),

		// App settings
		Timezone:    getEnv("TIMEZONE", "Europe/Moscow"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
		Environment: getEnv("ENVIRONMENT", "production"),

		// Rate limits
		ProDailyLimit:   getEnvInt("PRO_DAILY_LIMIT", 5),
		FlashDailyLimit: getEnvInt("FLASH_DAILY_LIMIT", 25),

		// LLM parameters
		LLMTemperature: getEnvFloat32("LLM_TEMPERATURE", 0.7),
		LLMTopP:        getEnvFloat32("LLM_TOP_P", 0.95),
		LLMTopK:        getEnvInt32("LLM_TOP_K", 40),
		LLMMaxTokens:   getEnvInt32("LLM_MAX_TOKENS", 8192),
	}

	// Validate configuration
	if err := validate(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

// validate checks if all required configuration values are set
func validate(cfg *models.BotConfig) error {
	if cfg.TelegramToken == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.TelegramUsername == "" {
		return fmt.Errorf("TELEGRAM_BOT_USERNAME is required")
	}
	if len(cfg.AllowedChatIDs) == 0 {
		return fmt.Errorf("TELEGRAM_ALLOWED_CHAT_IDS is required (comma-separated list of chat IDs)")
	}
	if cfg.GeminiAPIKey == "" {
		return fmt.Errorf("GEMINI_API_KEY is required")
	}
	if cfg.SupabaseURL == "" {
		return fmt.Errorf("SUPABASE_URL is required")
	}
	if cfg.SupabaseKey == "" {
		return fmt.Errorf("SUPABASE_KEY is required")
	}

	// Validate positive values
	if cfg.ProDailyLimit <= 0 {
		return fmt.Errorf("PRO_DAILY_LIMIT must be positive, got %d", cfg.ProDailyLimit)
	}
	if cfg.FlashDailyLimit <= 0 {
		return fmt.Errorf("FLASH_DAILY_LIMIT must be positive, got %d", cfg.FlashDailyLimit)
	}
	if cfg.GeminiTimeout <= 0 {
		return fmt.Errorf("GEMINI_TIMEOUT must be positive, got %d", cfg.GeminiTimeout)
	}
	if cfg.SupabaseTimeout <= 0 {
		return fmt.Errorf("SUPABASE_TIMEOUT must be positive, got %d", cfg.SupabaseTimeout)
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[cfg.LogLevel] {
		return fmt.Errorf("LOG_LEVEL must be one of: debug, info, warn, error; got %s", cfg.LogLevel)
	}

	return nil
}

// getEnv retrieves environment variable or returns default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt retrieves environment variable as integer or returns default value
func getEnvInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}

	return value
}

// getEnvInt64 retrieves environment variable as int64 or returns default value
func getEnvInt64(key string, defaultValue int64) int64 {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		return defaultValue
	}

	return value
}

// getEnvInt64List retrieves environment variable as a comma-separated list of int64
func getEnvInt64List(key string, defaultValue []int64) []int64 {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	parts := strings.Split(valueStr, ",")
	result := make([]int64, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		value, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			// Skip invalid values
			continue
		}

		result = append(result, value)
	}

	return result
}

// getEnvFloat32 retrieves environment variable as float32 or returns default value
func getEnvFloat32(key string, defaultValue float32) float32 {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseFloat(valueStr, 32)
	if err != nil {
		return defaultValue
	}
	return float32(value)
}

// getEnvInt32 retrieves environment variable as int32 or returns default value
func getEnvInt32(key string, defaultValue int32) int32 {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseInt(valueStr, 10, 32)
	if err != nil {
		return defaultValue
	}
	return int32(value)
}
