package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

func TestAdmins_UnansweredFlow(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	var bizID int64
	apiKey := fmt.Sprintf("test-%d", time.Now().UnixNano())
	if err := pool.QueryRow(ctx,
		`INSERT INTO businesses (name, api_key) VALUES ($1, $2) RETURNING id`, "biz", apiKey,
	).Scan(&bizID); err != nil {
		t.Fatalf("seed business: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id = $1`, bizID) })

	var convID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO conversations (business_id, channel, external_user_id, external_user_name, question, was_answered)
		VALUES ($1, 'telegram', '555', 'Aung (@aungshop)', 'do you deliver?', false)
		RETURNING id`, bizID,
	).Scan(&convID); err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	var queueID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO unanswered_queue (conversation_id, question) VALUES ($1, 'do you deliver?') RETURNING id`, convID,
	).Scan(&queueID); err != nil {
		t.Fatalf("seed queue: %v", err)
	}

	admins := NewAdmins(pool)

	// PendingUnanswered includes the customer's name.
	pending, err := admins.PendingUnanswered(ctx, bizID, 10)
	if err != nil {
		t.Fatalf("PendingUnanswered: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != queueID || pending[0].UserName != "Aung (@aungshop)" {
		t.Fatalf("pending wrong: %+v", pending)
	}

	// GetUnanswered returns the reply target.
	target, err := admins.GetUnanswered(ctx, bizID, queueID)
	if err != nil {
		t.Fatalf("GetUnanswered: %v", err)
	}
	if target.Channel != core.ChannelTelegram || target.ReplyTo != "555" {
		t.Fatalf("target wrong: %+v", target)
	}

	// Tenant isolation: another business can't fetch this item.
	if _, err := admins.GetUnanswered(ctx, bizID+999999, queueID); err == nil {
		t.Error("expected error fetching another business's item")
	}

	// Resolve → no longer pending.
	if err := admins.ResolveUnanswered(ctx, bizID, queueID); err != nil {
		t.Fatalf("ResolveUnanswered: %v", err)
	}
	pending, _ = admins.PendingUnanswered(ctx, bizID, 10)
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after resolve, got %d", len(pending))
	}
}
