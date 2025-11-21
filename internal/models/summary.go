package models

import "time"

// DailySummary represents a generated daily chat summary
type DailySummary struct {
	ID                 int64     `json:"id"`
	ChatID             int64     `json:"chat_id"`
	Date               string    `json:"date"` // Format: YYYY-MM-DD in Moscow timezone
	SummaryText        string    `json:"summary_text"`
	MostActiveUserID   int64     `json:"most_active_user_id,omitempty"`
	MostActiveUsername string    `json:"most_active_username,omitempty"`
	MessageCount       int       `json:"message_count"`
	CreatedAt          time.Time `json:"created_at"`
}

// UserMessageCount represents message count statistics for a user
type UserMessageCount struct {
	UserID       int64  `json:"user_id"`
	Username     string `json:"username,omitempty"`
	FirstName    string `json:"first_name,omitempty"`
	MessageCount int    `json:"message_count"`
}

// SummaryRequest represents a request to generate a summary
type SummaryRequest struct {
	ChatID   int64
	Date     string // Format: YYYY-MM-DD
	Messages []ChatMessage
}

// SummaryResult represents the result of summary generation
type SummaryResult struct {
	Topics         []string
	MostActiveUser *UserMessageCount
	MessageCount   int
	Error          error
}
