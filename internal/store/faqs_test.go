package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// fakeEmbedder maps text to a basis vector by keyword, so similarity is
// deterministic and testable without a real embedding model.
type fakeEmbedder struct{}

func (fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	idx := 2 // default bucket
	switch {
	case strings.Contains(text, "delivery"):
		idx = 0
	case strings.Contains(text, "refund"):
		idx = 1
	}
	v := make([]float32, 768)
	v[idx] = 1
	return v, nil
}

func TestFAQs_SearchRanksBySimilarityAndIsolatesTenants(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	newBiz := func() int64 {
		var id int64
		key := fmt.Sprintf("test-%d", time.Now().UnixNano())
		if err := pool.QueryRow(ctx,
			`INSERT INTO businesses (name, api_key) VALUES ($1, $2) RETURNING id`,
			"biz", key,
		).Scan(&id); err != nil {
			t.Fatalf("seed business: %v", err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id = $1`, id) })
		return id
	}

	bizA := newBiz()
	bizB := newBiz()

	faqs := NewFAQs(pool, fakeEmbedder{})

	// Business A has two FAQs.
	if _, err := faqs.Create(ctx, bizA, "what are your delivery times", "1-2 days", "shipping"); err != nil {
		t.Fatalf("create delivery faq: %v", err)
	}
	if _, err := faqs.Create(ctx, bizA, "refund policy", "30 days", "billing"); err != nil {
		t.Fatalf("create refund faq: %v", err)
	}
	// Business B has its own delivery FAQ that must never leak into A's results.
	if _, err := faqs.Create(ctx, bizB, "delivery for business B", "SECRET-B", ""); err != nil {
		t.Fatalf("create biz B faq: %v", err)
	}

	matches, err := faqs.Search(ctx, bizA, "how long is delivery", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}

	// Best match is the delivery FAQ, at near-perfect similarity.
	if matches[0].Answer != "1-2 days" {
		t.Errorf("top answer = %q, want %q", matches[0].Answer, "1-2 days")
	}
	if matches[0].Score < 0.99 {
		t.Errorf("top score = %f, want >= 0.99", matches[0].Score)
	}

	// Tenant isolation: none of business B's FAQs appear.
	for _, m := range matches {
		if m.Answer == "SECRET-B" {
			t.Fatalf("tenant isolation breach: business B's FAQ leaked into A's results")
		}
	}

	// Menu queries: categories in insertion order, list, and scoped get.
	cats, err := faqs.Categories(ctx, bizA)
	if err != nil {
		t.Fatalf("Categories: %v", err)
	}
	if len(cats) != 2 || cats[0] != "shipping" || cats[1] != "billing" {
		t.Errorf("categories = %v, want [shipping billing] in insertion order", cats)
	}

	list, err := faqs.ListByCategory(ctx, bizA, "shipping")
	if err != nil {
		t.Fatalf("ListByCategory: %v", err)
	}
	if len(list) != 1 || list[0].Answer != "1-2 days" {
		t.Errorf("list = %+v", list)
	}

	got, err := faqs.GetByID(ctx, bizA, list[0].ID)
	if err != nil || got.Question != "what are your delivery times" {
		t.Errorf("GetByID = %+v, err=%v", got, err)
	}
	// Scoped get: business B cannot fetch A's FAQ.
	if _, err := faqs.GetByID(ctx, bizB, list[0].ID); err == nil {
		t.Error("expected error fetching another business's FAQ")
	}
}
