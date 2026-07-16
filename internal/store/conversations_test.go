package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

// testPool connects to the integration test database, applying migrations.
// It skips the test when ASKDESK_TEST_DATABASE_URL is not set.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ASKDESK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set ASKDESK_TEST_DATABASE_URL to run store integration tests")
	}
	if err := Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestConversations_LogAndEnqueue(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// Seed a business (unique api_key per run); cascade cleans up children.
	var bizID int64
	apiKey := fmt.Sprintf("test-%d", time.Now().UnixNano())
	if err := pool.QueryRow(ctx,
		`INSERT INTO businesses (name, api_key) VALUES ($1, $2) RETURNING id`,
		"test biz", apiKey,
	).Scan(&bizID); err != nil {
		t.Fatalf("seed business: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id = $1`, bizID) })

	store := NewConversations(pool)

	id, err := store.LogConversation(ctx, core.ConversationRecord{
		BusinessID:  bizID,
		Channel:     core.ChannelTelegram,
		UserID:      "user-1",
		Question:    "do you deliver?",
		AIAnswer:    "Yes, we deliver.",
		Confidence:  0.91,
		WasAnswered: true,
	})
	if err != nil {
		t.Fatalf("LogConversation: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero conversation id")
	}

	var got int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM conversations WHERE id = $1`, id).Scan(&got); err != nil {
		t.Fatalf("count conversations: %v", err)
	}
	if got != 1 {
		t.Errorf("conversation rows = %d, want 1", got)
	}

	if err := store.EnqueueUnanswered(ctx, id, "do you deliver?"); err != nil {
		t.Fatalf("EnqueueUnanswered: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM unanswered_queue WHERE conversation_id = $1`, id).Scan(&got); err != nil {
		t.Fatalf("count unanswered: %v", err)
	}
	if got != 1 {
		t.Errorf("unanswered rows = %d, want 1", got)
	}
}
