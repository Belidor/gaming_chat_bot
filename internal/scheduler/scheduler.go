package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/telegram-llm-bot/internal/models"
	"github.com/telegram-llm-bot/internal/storage"
	"github.com/telegram-llm-bot/internal/summary"
)

// SummaryCallback is a function that sends the summary to a chat
type SummaryCallback func(chatID int64, summaryText string) error

// Scheduler handles scheduled tasks like daily summaries and RAG sync
type Scheduler struct {
	storage         *storage.Client
	generator       *summary.Generator
	config          *models.BotConfig
	summaryCallback SummaryCallback
	syncJob         *SyncJob
	logger          zerolog.Logger
	timezone        *time.Location
}

// NewScheduler creates a new scheduler
func NewScheduler(
	storage *storage.Client,
	generator *summary.Generator,
	config *models.BotConfig,
	summaryCallback SummaryCallback,
	syncJob *SyncJob,
	logger zerolog.Logger,
) (*Scheduler, error) {
	// Load timezone
	loc, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return nil, fmt.Errorf("failed to load timezone %s: %w", config.Timezone, err)
	}

	return &Scheduler{
		storage:         storage,
		generator:       generator,
		config:          config,
		summaryCallback: summaryCallback,
		syncJob:         syncJob,
		logger:          logger.With().Str("component", "scheduler").Logger(),
		timezone:        loc,
	}, nil
}

// Start starts the scheduler
func (s *Scheduler) Start(ctx context.Context) error {
	s.logger.Info().Msg("Starting scheduler...")

	// Calculate time until next 7 AM for summaries
	nextSummaryRun := s.calculateNextRun(7, 0)
	s.logger.Info().
		Time("next_summary_run", nextSummaryRun).
		Dur("wait_duration", time.Until(nextSummaryRun)).
		Msg("Scheduled next daily summary")

	// Calculate time until next 3 AM for RAG sync
	nextSyncRun := s.calculateNextRun(3, 0)
	s.logger.Info().
		Time("next_sync_run", nextSyncRun).
		Dur("wait_duration", time.Until(nextSyncRun)).
		Msg("Scheduled next RAG sync")

	// Start goroutine for summary scheduling
	go s.runSummaryScheduler(ctx, nextSummaryRun)

	// Start goroutine for sync scheduling
	go s.runSyncScheduler(ctx, nextSyncRun)

	s.logger.Info().Msg("Scheduler started and running")

	// Wait for context cancellation
	<-ctx.Done()
	s.logger.Info().Msg("Scheduler stopped")
	return ctx.Err()
}

// runSummaryScheduler handles daily summary scheduling
func (s *Scheduler) runSummaryScheduler(ctx context.Context, nextRun time.Time) {
	// Initial wait until first run
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Until(nextRun)):
		s.runDailySummaries(ctx)
	}

	// Create ticker for subsequent runs (every 24 hours)
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runDailySummaries(ctx)
		}
	}
}

// runSyncScheduler handles RAG sync scheduling
func (s *Scheduler) runSyncScheduler(ctx context.Context, nextRun time.Time) {
	// Initial wait until first run
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Until(nextRun)):
		s.runRAGSync(ctx)
	}

	// Create ticker for subsequent runs (every 24 hours)
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runRAGSync(ctx)
		}
	}
}

// calculateNextRun calculates the next run time for a specific hour and minute
func (s *Scheduler) calculateNextRun(hour, minute int) time.Time {
	now := time.Now().In(s.timezone)

	// Set to specified time today
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, s.timezone)

	// If it's already past the time today, schedule for tomorrow
	if now.After(next) || now.Equal(next) {
		next = next.AddDate(0, 0, 1)
	}

	return next
}

// runRAGSync executes RAG synchronization
func (s *Scheduler) runRAGSync(ctx context.Context) {
	s.logger.Info().Msg("Starting scheduled RAG sync")

	if s.syncJob == nil {
		s.logger.Warn().Msg("Sync job not configured, skipping RAG sync")
		return
	}

	if err := s.syncJob.Run(ctx); err != nil {
		s.logger.Error().
			Err(err).
			Msg("Scheduled RAG sync failed")
	} else {
		s.logger.Info().Msg("Scheduled RAG sync completed successfully")
	}
}

// runDailySummaries generates and sends summaries for all allowed chats
func (s *Scheduler) runDailySummaries(ctx context.Context) {
	s.logger.Info().Msg("Running daily summaries for all chats")

	// Get yesterday's date in Moscow timezone
	now := time.Now().In(s.timezone)
	yesterday := now.AddDate(0, 0, -1)
	dateStr := yesterday.Format("2006-01-02")

	s.logger.Info().
		Str("date", dateStr).
		Int("chat_count", len(s.config.AllowedChatIDs)).
		Msg("Generating summaries for yesterday")

	// Process each allowed chat
	for _, chatID := range s.config.AllowedChatIDs {
		// Use a separate goroutine for each chat to avoid blocking
		go func(cid int64) {
			if err := s.processChatSummary(ctx, cid, dateStr); err != nil {
				s.logger.Error().
					Err(err).
					Int64("chat_id", cid).
					Str("date", dateStr).
					Msg("Failed to process chat summary")
			}
		}(chatID)
	}
}

// processChatSummary generates and sends summary for a specific chat
func (s *Scheduler) processChatSummary(ctx context.Context, chatID int64, date string) error {
	return s.processChatSummaryWithForce(ctx, chatID, date, false)
}

// processChatSummaryWithForce generates and sends summary with optional force regeneration
func (s *Scheduler) processChatSummaryWithForce(ctx context.Context, chatID int64, date string, force bool) error {
	logger := s.logger.With().Int64("chat_id", chatID).Str("date", date).Bool("force", force).Logger()

	logger.Info().Msg("Processing daily summary")

	// Check if summary already exists (avoid duplicates on restart)
	if !force {
		exists, err := s.storage.SummaryExistsForDate(ctx, chatID, date)
		if err != nil {
			return fmt.Errorf("failed to check if summary exists: %w", err)
		}

		if exists {
			logger.Info().Msg("Summary already exists for this date, skipping")
			return nil
		}
	} else {
		logger.Info().Msg("Force flag set, will regenerate summary if exists")
	}

	// Get messages for the date
	messages, err := s.storage.GetMessagesForDate(ctx, chatID, date)
	if err != nil {
		return fmt.Errorf("failed to get messages: %w", err)
	}

	// Skip if no messages
	if len(messages) == 0 {
		logger.Info().Msg("No messages for this date, skipping summary")
		return nil
	}

	loc := s.timezone
	firstMsgMoscow := messages[0].CreatedAt.In(loc)
	lastMsgMoscow := messages[len(messages)-1].CreatedAt.In(loc)
	allMatch := true
	for _, msg := range messages {
		if msg.CreatedAt.In(loc).Format("2006-01-02") != date {
			allMatch = false
			break
		}
	}

	logger.Info().
		Int("message_count", len(messages)).
		Str("first_message_moscow", firstMsgMoscow.Format(time.RFC3339)).
		Str("last_message_moscow", lastMsgMoscow.Format(time.RFC3339)).
		Bool("all_messages_match_date", allMatch).
		Msg("Retrieved messages for summary (Moscow time)")

	// Get most active user
	mostActiveUser, err := s.storage.GetMostActiveUser(ctx, chatID, date)
	if err != nil {
		return fmt.Errorf("failed to get most active user: %w", err)
	}

	// Generate summary using LLM
	result, err := s.generator.GenerateSummary(ctx, messages, date)
	if err != nil {
		return fmt.Errorf("failed to generate summary: %w", err)
	}

	// Format summary message
	summaryText := s.formatSummaryMessage(date, result.Topics, mostActiveUser)

	// Save to database
	dailySummary := &models.DailySummary{
		ChatID:       chatID,
		Date:         date,
		SummaryText:  summaryText,
		MessageCount: len(messages),
	}

	if mostActiveUser != nil {
		dailySummary.MostActiveUserID = mostActiveUser.UserID
		dailySummary.MostActiveUsername = mostActiveUser.Username
	}

	if err := s.storage.SaveDailySummary(ctx, dailySummary); err != nil {
		return fmt.Errorf("failed to save summary: %w", err)
	}

	// Send summary to chat
	if s.summaryCallback != nil {
		if err := s.summaryCallback(chatID, summaryText); err != nil {
			return fmt.Errorf("failed to send summary: %w", err)
		}
	}

	logger.Info().
		Int("topic_count", len(result.Topics)).
		Int("message_count", len(messages)).
		Msg("Daily summary completed successfully")

	return nil
}

// GenerateSummaryForYesterday generates summary for yesterday for a specific chat (used for manual /summary command)
func (s *Scheduler) GenerateSummaryForYesterday(ctx context.Context, chatID int64) error {
	// Get yesterday's date in Moscow timezone
	now := time.Now().In(s.timezone)
	yesterday := now.AddDate(0, 0, -1)
	dateStr := yesterday.Format("2006-01-02")

	s.logger.Info().
		Int64("chat_id", chatID).
		Str("date", dateStr).
		Msg("Manual summary generation requested")

	// Use force=true for manual requests to allow regeneration
	return s.processChatSummaryWithForce(ctx, chatID, dateStr, true)
}

// escapeMarkdownV1 escapes special characters for Telegram Markdown V1
// Markdown V1 requires escaping: _ * [ ] ( ) ~ ` > # + - = | { } . !
func escapeMarkdownV1(text string) string {
	// For Markdown V1, we mainly need to escape underscores in usernames
	// Other special chars are less common in usernames
	replacements := map[string]string{
		"_": "\\_",
		"*": "\\*",
		"[": "\\[",
		"`": "\\`",
	}

	result := text
	for old, new := range replacements {
		result = strings.ReplaceAll(result, old, new)
	}
	return result
}

// formatSummaryMessage formats the summary into a nice Telegram message
func (s *Scheduler) formatSummaryMessage(date string, topics []string, mostActiveUser *models.UserMessageCount) string {
	// Parse date for prettier display
	t, err := time.Parse("2006-01-02", date)
	var dateDisplay string
	if err == nil {
		// Format as "20 ноября"
		months := []string{
			"января", "февраля", "марта", "апреля", "мая", "июня",
			"июля", "августа", "сентября", "октября", "ноября", "декабря",
		}
		dateDisplay = fmt.Sprintf("%d %s", t.Day(), months[t.Month()-1])
	} else {
		dateDisplay = date
	}

	var message string
	message = fmt.Sprintf("📊 *Саммари за %s*\n\n", dateDisplay)

	if len(topics) > 0 {
		message += "*Основные темы обсуждения:*\n"
		for _, topic := range topics {
			message += topic + "\n"
		}
	} else {
		message += "*В этот день не было активных обсуждений*\n"
	}

	if mostActiveUser != nil {
		username := mostActiveUser.Username
		if username == "" {
			username = mostActiveUser.FirstName
		}
		if username == "" {
			username = fmt.Sprintf("User%d", mostActiveUser.UserID)
		}

		// Escape special Markdown characters in username
		escapedUsername := escapeMarkdownV1(username)

		// Format message count with proper Russian pluralization
		var msgWord string
		count := mostActiveUser.MessageCount
		if count%10 == 1 && count%100 != 11 {
			msgWord = "сообщение"
		} else if (count%10 >= 2 && count%10 <= 4) && (count%100 < 10 || count%100 >= 20) {
			msgWord = "сообщения"
		} else {
			msgWord = "сообщений"
		}

		message += fmt.Sprintf("\n*Вчера больше всех пиздел:* @%s (%d %s)",
			escapedUsername, count, msgWord)
	}

	return message
}
