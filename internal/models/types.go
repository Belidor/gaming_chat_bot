package models

import "time"

// ModelType represents the type of LLM model
type ModelType string

const (
	// ModelPro represents Gemini 2.5 Pro model
	// Used for complex tasks requiring deeper reasoning
	// See current rate limits: https://ai.google.dev/pricing
	ModelPro ModelType = "gemini-2.5-pro"

	// ModelFlash represents Gemini 2.0 Flash model
	// Used for fast responses with good quality
	// See current rate limits: https://ai.google.dev/pricing
	ModelFlash ModelType = "gemini-2.0-flash"
)

// String returns string representation of ModelType
func (m ModelType) String() string {
	return string(m)
}

// RequestLog represents a log entry for a user request
type RequestLog struct {
	ID              int64     `json:"id"`
	UserID          int64     `json:"user_id"`
	Username        string    `json:"username,omitempty"`
	FirstName       string    `json:"first_name,omitempty"`
	ChatID          int64     `json:"chat_id"`
	RequestText     string    `json:"request_text"`
	ResponseText    string    `json:"response_text"`
	ModelUsed       string    `json:"model_used"`
	ResponseLength  int       `json:"response_length"`
	ExecutionTimeMs int       `json:"execution_time_ms"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// DailyLimit represents daily usage limits for a user
type DailyLimit struct {
	ID                 int64     `json:"id"`
	UserID             int64     `json:"user_id"`
	Date               string    `json:"date"` // Format: YYYY-MM-DD in Moscow timezone
	ProRequestsCount   int       `json:"pro_requests_count"`
	FlashRequestsCount int       `json:"flash_requests_count"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// UserStats represents statistics for a specific user
type UserStats struct {
	UserID             int64  `json:"user_id"`
	Username           string `json:"username,omitempty"`
	FirstName          string `json:"first_name,omitempty"`
	ProRequestsUsed    int    `json:"pro_requests_used"`
	ProRequestsLimit   int    `json:"pro_requests_limit"`
	FlashRequestsUsed  int    `json:"flash_requests_used"`
	FlashRequestsLimit int    `json:"flash_requests_limit"`
	TotalRequests      int64  `json:"total_requests"`
	ResetsInHours      int    `json:"resets_in_hours"`
}

// LLMRequest represents a request to LLM
type LLMRequest struct {
	UserID      int64
	Username    string
	FirstName   string
	ChatID      int64
	Text        string
	ModelType   ModelType
	TimeoutSecs int
}

// LLMResponse represents a response from LLM
type LLMResponse struct {
	Text            string
	ModelUsed       string
	Length          int
	ExecutionTimeMs int
	Error           error
}

// RateLimitResult represents the result of rate limit check
type RateLimitResult struct {
	Allowed        bool
	ModelToUse     ModelType
	ProRemaining   int
	FlashRemaining int
	Message        string
}

// BotConfig represents bot configuration
type BotConfig struct {
	// Telegram settings
	TelegramToken    string
	TelegramUsername string
	AllowedChatIDs   []int64 // List of allowed chat IDs (supports multiple chats)

	// Gemini API settings
	GeminiAPIKey  string
	GeminiTimeout int

	// Supabase settings
	SupabaseURL     string
	SupabaseKey     string
	SupabaseTimeout int

	// App settings
	Timezone    string
	LogLevel    string
	Environment string

	// Rate limits
	ProDailyLimit   int
	FlashDailyLimit int

	// LLM Generation Parameters
	LLMTemperature float32
	LLMTopP        float32
	LLMTopK        int32
	LLMMaxTokens   int32
}

// IsAllowedChat checks if the given chat ID is in the allowed list
func (c *BotConfig) IsAllowedChat(chatID int64) bool {
	for _, allowedID := range c.AllowedChatIDs {
		if allowedID == chatID {
			return true
		}
	}
	return false
}
