package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/telegram-llm-bot/internal/models"
	"github.com/telegram-llm-bot/internal/storage"
)

// Limiter manages rate limits for users
type Limiter struct {
	storage         *storage.Client
	timezone        *time.Location
	proDailyLimit   int
	flashDailyLimit int
	logger          zerolog.Logger
}

// NewLimiter creates a new rate limiter
func NewLimiter(storage *storage.Client, timezone string, proLimit, flashLimit int, logger zerolog.Logger) (*Limiter, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("failed to load timezone %s: %w", timezone, err)
	}

	return &Limiter{
		storage:         storage,
		timezone:        loc,
		proDailyLimit:   proLimit,
		flashDailyLimit: flashLimit,
		logger:          logger.With().Str("component", "ratelimit").Logger(),
	}, nil
}

// CheckLimit checks if user can make a request and determines which model to use
func (l *Limiter) CheckLimit(ctx context.Context, userID int64) (*models.RateLimitResult, error) {
	// Get current date in Moscow timezone
	now := time.Now().In(l.timezone)
	dateStr := now.Format("2006-01-02")

	// Get user's daily limits
	limits, err := l.storage.GetDailyLimit(ctx, userID, dateStr)
	if err != nil {
		l.logger.Error().
			Err(err).
			Int64("user_id", userID).
			Str("date", dateStr).
			Msg("Failed to get daily limit")
		return nil, fmt.Errorf("failed to check rate limit: %w", err)
	}

	proRemaining := l.proDailyLimit - limits.ProRequestsCount
	flashRemaining := l.flashDailyLimit - limits.FlashRequestsCount

	l.logger.Debug().
		Int64("user_id", userID).
		Int("pro_used", limits.ProRequestsCount).
		Int("pro_remaining", proRemaining).
		Int("flash_used", limits.FlashRequestsCount).
		Int("flash_remaining", flashRemaining).
		Msg("Checking rate limit")

	// Check if user has exceeded both limits
	if proRemaining <= 0 && flashRemaining <= 0 {
		hoursUntilReset := l.hoursUntilMidnight(now)
		return &models.RateLimitResult{
			Allowed:        false,
			ModelToUse:     "",
			ProRemaining:   0,
			FlashRemaining: 0,
			Message: fmt.Sprintf(
				"🚫 Вы исчерпали дневной лимит запросов.\n\n"+
					"Лимиты сбросятся через %d ч.\n"+
					"Pro: %d/%d\nFlash: %d/%d",
				hoursUntilReset,
				limits.ProRequestsCount, l.proDailyLimit,
				limits.FlashRequestsCount, l.flashDailyLimit,
			),
		}, nil
	}

	// Determine which model to use
	var modelToUse models.ModelType
	if proRemaining > 0 {
		modelToUse = models.ModelPro
	} else {
		modelToUse = models.ModelFlash
	}

	return &models.RateLimitResult{
		Allowed:        true,
		ModelToUse:     modelToUse,
		ProRemaining:   proRemaining,
		FlashRemaining: flashRemaining,
		Message:        "",
	}, nil
}

// IncrementUsage increments the usage count for a user
func (l *Limiter) IncrementUsage(ctx context.Context, userID int64, modelType models.ModelType) error {
	// Get current date in Moscow timezone
	now := time.Now().In(l.timezone)
	dateStr := now.Format("2006-01-02")

	err := l.storage.IncrementLimit(ctx, userID, dateStr, modelType)
	if err != nil {
		l.logger.Error().
			Err(err).
			Int64("user_id", userID).
			Str("model", string(modelType)).
			Msg("Failed to increment usage")
		return fmt.Errorf("failed to increment usage: %w", err)
	}

	l.logger.Debug().
		Int64("user_id", userID).
		Str("model", string(modelType)).
		Str("date", dateStr).
		Msg("Usage incremented")

	return nil
}

// GetUserStats returns statistics for a user
func (l *Limiter) GetUserStats(ctx context.Context, userID int64, username, firstName string) (*models.UserStats, error) {
	// Get current date in Moscow timezone
	now := time.Now().In(l.timezone)
	dateStr := now.Format("2006-01-02")

	// Get daily limits
	limits, err := l.storage.GetDailyLimit(ctx, userID, dateStr)
	if err != nil {
		l.logger.Error().
			Err(err).
			Int64("user_id", userID).
			Msg("Failed to get user stats")
		return nil, fmt.Errorf("failed to get user stats: %w", err)
	}

	// Get total requests (all time)
	totalRequests, err := l.storage.GetUserTotalRequests(ctx, userID)
	if err != nil {
		l.logger.Warn().
			Err(err).
			Int64("user_id", userID).
			Msg("Failed to get total requests, using 0")
		totalRequests = 0
	}

	hoursUntilReset := l.hoursUntilMidnight(now)

	return &models.UserStats{
		UserID:             userID,
		Username:           username,
		FirstName:          firstName,
		ProRequestsUsed:    limits.ProRequestsCount,
		ProRequestsLimit:   l.proDailyLimit,
		FlashRequestsUsed:  limits.FlashRequestsCount,
		FlashRequestsLimit: l.flashDailyLimit,
		TotalRequests:      totalRequests,
		ResetsInHours:      hoursUntilReset,
	}, nil
}

// hoursUntilMidnight calculates hours until midnight in the timezone
func (l *Limiter) hoursUntilMidnight(now time.Time) int {
	// Get midnight of next day
	midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, l.timezone)
	duration := midnight.Sub(now)
	hours := int(duration.Hours())

	// If less than 1 hour, show at least 1
	if hours < 1 {
		hours = 1
	}

	return hours
}
