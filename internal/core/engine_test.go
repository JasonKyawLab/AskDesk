package core

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

// --- fakes ---

type fakeRetriever struct {
	matches []Match
	err     error
}

func (f *fakeRetriever) Search(context.Context, int64, string, int) ([]Match, error) {
	return f.matches, f.err
}

type fakeAI struct {
	answer string
	err    error
}

func (f *fakeAI) GenerateReply(context.Context, string, []Match) (string, error) {
	return f.answer, f.err
}

type fakeStore struct {
	logged     ConversationRecord
	enqueued   bool
	logErr     error
	enqueueErr error
}

func (f *fakeStore) LogConversation(_ context.Context, rec ConversationRecord) (int64, error) {
	f.logged = rec
	return 1, f.logErr
}

func (f *fakeStore) EnqueueUnanswered(context.Context, int64, string) error {
	f.enqueued = true
	return f.enqueueErr
}

func newTestEngine(r Retriever, ai AIProvider, s ConversationStore) *Engine {
	return NewEngine(r, ai, s, slog.New(slog.NewTextHandler(io.Discard, nil)), "fallback")
}

// --- tests ---

func TestGenerateCustomerReply_HighConfidence(t *testing.T) {
	retriever := &fakeRetriever{matches: []Match{{FAQID: 42, Answer: "Yes we do.", Score: 0.91}}}
	store := &fakeStore{}
	engine := newTestEngine(retriever, &fakeAI{answer: "Yes, we deliver."}, store)

	reply, err := engine.GenerateCustomerReply(context.Background(), Message{BusinessID: 1, Text: "do you deliver?"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reply.Answered {
		t.Errorf("Answered = false, want true for score 0.91")
	}
	if reply.MatchedFAQID == nil || *reply.MatchedFAQID != 42 {
		t.Errorf("MatchedFAQID = %v, want 42", reply.MatchedFAQID)
	}
	if store.enqueued {
		t.Errorf("confident answer should not be enqueued as unanswered")
	}
	if !store.logged.WasAnswered {
		t.Errorf("logged record should be marked answered")
	}
}

func TestGenerateCustomerReply_LowConfidenceIsFlagged(t *testing.T) {
	retriever := &fakeRetriever{matches: []Match{{FAQID: 7, Answer: "Maybe.", Score: 0.30}}}
	store := &fakeStore{}
	engine := newTestEngine(retriever, &fakeAI{answer: "I'm not sure, let me check."}, store)

	reply, err := engine.GenerateCustomerReply(context.Background(), Message{BusinessID: 1, Text: "obscure question"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply.Answered {
		t.Errorf("Answered = true, want false for score 0.30")
	}
	if !store.enqueued {
		t.Errorf("low-confidence answer should be enqueued as unanswered")
	}
}

func TestGenerateCustomerReply_NoMatches(t *testing.T) {
	store := &fakeStore{}
	engine := newTestEngine(&fakeRetriever{matches: nil}, &fakeAI{answer: "Sorry, I don't know."}, store)

	reply, err := engine.GenerateCustomerReply(context.Background(), Message{BusinessID: 1, Text: "???"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply.Answered || reply.MatchedFAQID != nil {
		t.Errorf("no matches should yield unanswered with no matched FAQ")
	}
	if !store.enqueued {
		t.Errorf("no-match answer should be enqueued")
	}
}

func TestGenerateCustomerReply_RetrieverError_Degrades(t *testing.T) {
	store := &fakeStore{}
	engine := newTestEngine(&fakeRetriever{err: errors.New("db down")}, &fakeAI{}, store)

	reply, err := engine.GenerateCustomerReply(context.Background(), Message{BusinessID: 1, Text: "hi"})
	if err != nil {
		t.Fatalf("expected graceful degrade, got error: %v", err)
	}
	if reply.Answered || reply.Text != "fallback" {
		t.Errorf("expected fallback reply, got %+v", reply)
	}
	if !store.enqueued {
		t.Error("degraded question should be enqueued as unanswered")
	}
}

func TestGenerateCustomerReply_AIError_Degrades(t *testing.T) {
	retriever := &fakeRetriever{matches: []Match{{FAQID: 1, Score: 0.9}}}
	store := &fakeStore{}
	engine := newTestEngine(retriever, &fakeAI{err: errors.New("all providers down")}, store)

	reply, err := engine.GenerateCustomerReply(context.Background(), Message{BusinessID: 1, Text: "hi"})
	if err != nil {
		t.Fatalf("expected graceful degrade, got error: %v", err)
	}
	if reply.Answered || reply.Text != "fallback" {
		t.Errorf("expected fallback reply, got %+v", reply)
	}
	if !store.enqueued {
		t.Error("degraded question should be enqueued as unanswered")
	}
}
