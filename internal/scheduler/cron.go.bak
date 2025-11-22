package scheduler

import (
	"context"
	"fmt"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
)

// Scheduler manages scheduled jobs
type Scheduler struct {
	cron    *cron.Cron
	syncJob *SyncJob
	logger  zerolog.Logger
}

// NewScheduler creates a new scheduler
func NewScheduler(syncJob *SyncJob, logger zerolog.Logger) *Scheduler {
	// Create cron with Moscow timezone
	location, err := getTimezone()
	if err != nil {
		logger.Warn().
			Err(err).
			Msg("Failed to load timezone, using UTC")
		location = nil
	}

	var c *cron.Cron
	if location != nil {
		c = cron.New(cron.WithLocation(location))
	} else {
		c = cron.New()
	}

	return &Scheduler{
		cron:    c,
		syncJob: syncJob,
		logger:  logger.With().Str("component", "scheduler").Logger(),
	}
}

// Start starts the scheduler
func (s *Scheduler) Start(ctx context.Context) error {
	// Schedule nightly sync at 03:00 Moscow time
	// Cron format: minute hour day month weekday
	// "0 3 * * *" = every day at 03:00
	_, err := s.cron.AddFunc("0 3 * * *", func() {
		s.logger.Info().Msg("Starting scheduled RAG sync")
		if err := s.syncJob.Run(context.Background()); err != nil {
			s.logger.Error().
				Err(err).
				Msg("Scheduled sync job failed")
		} else {
			s.logger.Info().Msg("Scheduled sync job completed successfully")
		}
	})

	if err != nil {
		return fmt.Errorf("failed to schedule sync job: %w", err)
	}

	s.cron.Start()
	s.logger.Info().Msg("Scheduler started, nightly sync scheduled for 03:00 MSK")

	// Wait for context cancellation
	<-ctx.Done()

	s.logger.Info().Msg("Stopping scheduler...")
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()
	s.logger.Info().Msg("Scheduler stopped")

	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.cron.Stop()
}
