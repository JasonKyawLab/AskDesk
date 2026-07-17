package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JasonKyawLab/AskDesk/internal/store"
)

type fakeMenuStore struct{}

func (fakeMenuStore) Categories(context.Context, int64) ([]string, error) {
	return []string{"Getting Started", "Pricing", "Products"}, nil
}

func (fakeMenuStore) ListByCategory(_ context.Context, _ int64, category string) ([]store.FAQ, error) {
	return []store.FAQ{
		{ID: 1, Question: "What is MiniPOS?", Answer: "A web POS.", Category: category},
		{ID: 2, Question: "Do I need to install anything?", Answer: "No.", Category: category},
	}, nil
}

func (fakeMenuStore) GetByID(context.Context, int64, int64) (store.FAQ, error) {
	return store.FAQ{ID: 1, Question: "What is MiniPOS?", Answer: "A web POS.", Category: "Getting Started"}, nil
}

type fakeMenuClient struct {
	sentText   string
	sentKb     Keyboard
	editedText string
	editedKb   Keyboard
	acked      bool
}

func (f *fakeMenuClient) SendMenu(_ context.Context, _ int64, text string, kb Keyboard) error {
	f.sentText, f.sentKb = text, kb
	return nil
}

func (f *fakeMenuClient) EditMenu(_ context.Context, _, _ int64, text string, kb Keyboard) error {
	f.editedText, f.editedKb = text, kb
	return nil
}

func (f *fakeMenuClient) AnswerCallback(context.Context, string) error {
	return nil
}

func menuHandler(sub *fakeSubmitter, client *fakeMenuClient) *Handler {
	return NewHandler(sub, fakeMenuStore{}, client, nil, nil, 1, "", discardLogger())
}

func post(h *Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/telegram", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMenu_GreetingShowsMenuNotAI(t *testing.T) {
	sub := &fakeSubmitter{}
	client := &fakeMenuClient{}
	h := menuHandler(sub, client)

	rec := post(h, `{"message":{"text":"hi","chat":{"id":5},"from":{"id":9}}}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if sub.called {
		t.Error("greeting must not reach the AI/submitter")
	}
	if !strings.Contains(client.sentText, "Welcome") {
		t.Errorf("expected welcome menu, got %q", client.sentText)
	}
	// Categories plus the ask row.
	if len(client.sentKb) == 0 {
		t.Fatal("expected keyboard rows")
	}
	last := client.sentKb[len(client.sentKb)-1]
	if last[0].Data != cbAsk {
		t.Errorf("last row should be the ask button, got %+v", last)
	}
}

func TestMenu_CategoryCallbackListsQuestions(t *testing.T) {
	sub := &fakeSubmitter{}
	client := &fakeMenuClient{}
	h := menuHandler(sub, client)

	post(h, `{"callback_query":{"id":"cb1","data":"c:Pricing","message":{"message_id":7,"chat":{"id":5}}}}`)

	if !strings.Contains(client.editedText, "Pricing") {
		t.Errorf("expected category screen, got %q", client.editedText)
	}
	if len(client.editedKb) != 3 { // 2 questions + main-menu row
		t.Fatalf("expected 3 rows, got %d", len(client.editedKb))
	}
	if client.editedKb[0][0].Data != "q:1" {
		t.Errorf("first question button = %+v", client.editedKb[0][0])
	}
}

func TestMenu_AnswerCallbackShowsAnswerWithNav(t *testing.T) {
	sub := &fakeSubmitter{}
	client := &fakeMenuClient{}
	h := menuHandler(sub, client)

	post(h, `{"callback_query":{"id":"cb2","data":"q:1","message":{"message_id":7,"chat":{"id":5}}}}`)

	if !strings.Contains(client.editedText, "A web POS.") {
		t.Errorf("expected answer text, got %q", client.editedText)
	}
	nav := client.editedKb[0]
	if nav[0].Data != "c:Getting Started" || nav[1].Data != cbMain {
		t.Errorf("nav row wrong: %+v", nav)
	}
}

func TestMenu_FreeTextStillGoesToSubmitter(t *testing.T) {
	sub := &fakeSubmitter{}
	client := &fakeMenuClient{}
	h := menuHandler(sub, client)

	post(h, `{"message":{"text":"can I export sales to excel?","chat":{"id":5},"from":{"id":9}}}`)

	if !sub.called {
		t.Error("free text should flow to the submitter (AI)")
	}
}

func TestMenu_DisabledFallsBackToSubmitter(t *testing.T) {
	sub := &fakeSubmitter{}
	h := NewHandler(sub, nil, nil, nil, nil, 1, "", discardLogger())

	post(h, `{"message":{"text":"hi","chat":{"id":5},"from":{"id":9}}}`)

	if !sub.called {
		t.Error("with menus disabled, greetings should flow to the submitter")
	}
}
