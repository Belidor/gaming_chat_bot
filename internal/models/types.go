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
	RAGContext  string    // Optional RAG context from chat history
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

// ChatMessage represents a message from chat (for RAG)
type ChatMessage struct {
	ID          int64     `json:"id"`
	MessageID   int64     `json:"message_id"`
	UserID      int64     `json:"user_id"`
	Username    string    `json:"username,omitempty"`
	FirstName   string    `json:"first_name,omitempty"`
	ChatID      int64     `json:"chat_id"`
	MessageText string    `json:"message_text"`
	Embedding   []float32 `json:"embedding,omitempty"`
	Indexed     bool      `json:"indexed"`
	CreatedAt   time.Time `json:"created_at"`
	IndexedAt   time.Time `json:"indexed_at,omitempty"`
	Similarity  float64   `json:"similarity,omitempty"` // Used in search results
}

// RAGResult represents the result of RAG search
type RAGResult struct {
	Context   string         `json:"context"`
	Messages  []*ChatMessage `json:"messages"`
	QueryUsed string         `json:"query_used"`
	Count     int            `json:"count"`
}

// RAGConfig represents RAG system configuration
type RAGConfig struct {
	Enabled             bool    `json:"enabled"`
	TopK                int     `json:"top_k"`                 // Number of results to return (default: 5)
	SimilarityThreshold float64 `json:"similarity_threshold"`  // Minimum similarity score (default: 0.8)
	MaxContextLength    int     `json:"max_context_length"`    // Max characters in context (default: 2000)
	EmbeddingsModel     string  `json:"embeddings_model"`      // Model for embeddings (default: text-embedding-004)
	EmbeddingsBatchSize int     `json:"embeddings_batch_size"` // Batch size for embeddings (default: 100)
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

	// RAG Configuration
	RAG RAGConfig
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
