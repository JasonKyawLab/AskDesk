package app

import (
	"context"
	"log/slog"
	"time"
)

// ConversationPurger deletes conversations older than a cutoff.
type ConversationPurger interface {
	PurgeOlderThan(ctx context.Context, days int) (int64, error)
}

// StartRetention launches a background job that deletes conversations older than
// retentionDays — once at startup, then daily. retentionDays <= 0 disables it
// (nothing is ever deleted), which is the default. Keeps the database bounded
// and honours per-deployment data-retention limits for privacy.
func StartRetention(ctx context.Context, p ConversationPurger, retentionDays int, log *slog.Logger) {
	if retentionDays <= 0 {
		return
	}
	log.Info("retention enabled", "delete_conversations_older_than_days", retentionDays)

	purge := func() {
		n, err := p.PurgeOlderThan(ctx, retentionDays)
		if err != nil {
			log.Error("retention purge failed", "error", err)
			return
		}
		if n > 0 {
			log.Info("retention purge", "deleted_conversations", n, "older_than_days", retentionDays)
		}
	}

	go func() {
		purge()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				purge()
			}
		}
	}()
}
