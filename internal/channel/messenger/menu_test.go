package messenger

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JasonKyawLab/AskDesk/internal/store"
)

type fakeMenuStore struct{ faqs []store.FAQ }

func (f fakeMenuStore) Categories(context.Context, int64) ([]string, error) {
	seen := map[string]bool{}
	var cats []string
	for _, q := range f.faqs {
		if q.Category != "" && !seen[q.Category] {
			seen[q.Category] = true
			cats = append(cats, q.Category)
		}
	}
	return cats, nil
}

func (f fakeMenuStore) ListByCategory(_ context.Context, _ int64, cat string) ([]store.FAQ, error) {
	var out []store.FAQ
	for _, q := range f.faqs {
		if q.Category == cat {
			out = append(out, q)
		}
	}
	return out, nil
}

func (f fakeMenuStore) GetByID(_ context.Context, _ int64, id int64) (store.FAQ, error) {
	for _, q := range f.faqs {
		if q.ID == id {
			return q, nil
		}
	}
	return store.FAQ{}, errors.New("not found")
}

// recordingClient captures what the menu renders.
type recordingClient struct {
	texts     []string
	quickText string
	quick     []QuickReply
	cards     []Card
}

func (c *recordingClient) SendMessage(_ context.Context, _, text string) error {
	c.texts = append(c.texts, text)
	return nil
}
func (c *recordingClient) SendQuickReplies(_ context.Context, _, text string, replies []QuickReply) error {
	c.quickText = text
	c.quick = replies
	return nil
}
func (c *recordingClient) SendCarousel(_ context.Context, _ string, cards []Card) error {
	c.cards = cards
	return nil
}

var menuFAQs = []store.FAQ{
	{ID: 1, Category: "Pricing", Question: "How much?", Answer: "Free."},
	{ID: 2, Category: "Pricing", Question: "Refunds?", Answer: "Within 7 days."},
	{ID: 3, Category: "Features", Question: "Offline?", Answer: "No."},
}

func newMenuHandler(rc *recordingClient) *Handler {
	return NewHandler(&fakeSubmitter{}, fakeMenuStore{faqs: menuFAQs}, rc, nil, 1, "", "v", discardLogger())
}

func post(h *Handler, body string) {
	req := httptest.NewRequest(http.MethodPost, "/webhook/messenger", strings.NewReader(body))
	h.ServeHTTP(httptest.NewRecorder(), req)
}

func TestMenu_GreetingShowsCategories(t *testing.T) {
	rc := &recordingClient{}
	post(newMenuHandler(rc), `{"entry":[{"messaging":[{"sender":{"id":"U"},"message":{"text":"hi"}}]}]}`)

	// Expect category chips + an "Ask" chip; no engine submit.
	if len(rc.quick) != 3 { // Pricing, Features, Ask
		t.Fatalf("quick replies = %d, want 3: %+v", len(rc.quick), rc.quick)
	}
	if rc.quick[0].Payload != prefixCat+"Pricing" {
		t.Errorf("first chip payload = %q", rc.quick[0].Payload)
	}
	if rc.quick[len(rc.quick)-1].Payload != payloadAsk {
		t.Errorf("last chip should be Ask, got %q", rc.quick[len(rc.quick)-1].Payload)
	}
}

func TestMenu_CategoryQuickReplyShowsCarousel(t *testing.T) {
	rc := &recordingClient{}
	post(newMenuHandler(rc), `{"entry":[{"messaging":[{"sender":{"id":"U"},"message":{"text":"x","quick_reply":{"payload":"CAT:Pricing"}}}]}]}`)

	if len(rc.cards) != 2 {
		t.Fatalf("cards = %d, want 2 (Pricing has 2 FAQs)", len(rc.cards))
	}
	if rc.cards[0].Title != "How much?" || rc.cards[0].Buttons[0].Payload != "FAQ:1" {
		t.Errorf("unexpected first card: %+v", rc.cards[0])
	}
}

func TestMenu_PostbackAnswerSendsFAQ(t *testing.T) {
	rc := &recordingClient{}
	post(newMenuHandler(rc), `{"entry":[{"messaging":[{"sender":{"id":"U"},"postback":{"payload":"FAQ:2"}}]}]}`)

	if !strings.Contains(rc.quickText, "Within 7 days.") {
		t.Errorf("answer text missing FAQ answer: %q", rc.quickText)
	}
	if rc.quick[0].Payload != prefixCat+"Pricing" {
		t.Errorf("first nav chip should go back to category, got %q", rc.quick[0].Payload)
	}
}

func TestMenu_FreeTextStillReachesEngine(t *testing.T) {
	rc := &recordingClient{}
	sub := &fakeSubmitter{}
	h := NewHandler(sub, fakeMenuStore{faqs: menuFAQs}, rc, nil, 1, "", "v", discardLogger())
	post(h, `{"entry":[{"messaging":[{"sender":{"id":"U"},"message":{"text":"do you support kbz pay?"}}]}]}`)

	if !sub.called {
		t.Error("a real question should be submitted to the engine, not the menu")
	}
	if len(rc.quick) != 0 || len(rc.cards) != 0 {
		t.Error("a real question must not trigger menu rendering")
	}
}
