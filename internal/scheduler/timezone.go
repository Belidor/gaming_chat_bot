package scheduler

import (
	"os"
	"time"
)

// getTimezone returns the configured timezone or UTC
func getTimezone() (*time.Location, error) {
	tz := os.Getenv("TIMEZONE")
	if tz == "" {
		tz = "Europe/Moscow"
	}

	location, err := time.LoadLocation(tz)
	if err != nil {
		return nil, err
	}

	return location, nil
}
