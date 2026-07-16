package core

import (
	"context"
	"fmt"
	"log/slog"
)

// Retriever finds the FAQs most relevant to a query for a business (RAG lookup).
// Results are ordered by descending Score.
type Retriever interface {
	Search(ctx context.Context, businessID int64, query string, limit int) ([]Match, error)
}

// AIProvider generates a natural-language answer grounded in the retrieved FAQs.
// Implementations form a failover chain, but the engine sees just one provider.
type AIProvider interface {
	GenerateReply(ctx context.Context, question string, context []Match) (string, error)
}

// ConversationStore persists each interaction and flags low-confidence ones.
type ConversationStore interface {
	LogConversation(ctx context.Context, rec ConversationRecord) (conversationID int64, err error)
	EnqueueUnanswered(ctx context.Context, conversationID int64, question string) error
}

// ConversationRecord is a single logged interaction (maps to the conversations table).
type ConversationRecord struct {
	BusinessID   int64
	Channel      Channel
	UserID       string
	Question     string
	MatchedFAQID *int64
	AIAnswer     string
	Confidence   float64
	WasAnswered  bool
}

const (
	// defaultTopK is how many FAQ matches to retrieve for context.
	defaultTopK = 4
	// defaultConfidenceThreshold is the minimum best-match score to treat an
	// answer as confident; below it, the question is flagged for an admin.
	defaultConfidenceThreshold = 0.75
)

// Engine is the shared reply engine. It is channel-agnostic and holds no state.
type Engine struct {
	retriever Retriever
	ai        AIProvider
	store     ConversationStore
	log       *slog.Logger
	threshold float64
}

// NewEngine constructs an Engine with the default confidence threshold.
func NewEngine(r Retriever, ai AIProvider, store ConversationStore, log *slog.Logger) *Engine {
	return &Engine{
		retriever: r,
		ai:        ai,
		store:     store,
		log:       log,
		threshold: defaultConfidenceThreshold,
	}
}

// GenerateCustomerReply is the single entrypoint every customer channel funnels
// into: RAG lookup, confidence check, AI generation, then logging. Low-confidence
// answers are still returned but flagged to the unanswered queue for an admin.
func (e *Engine) GenerateCustomerReply(ctx context.Context, msg Message) (Reply, error) {
	matches, err := e.retriever.Search(ctx, msg.BusinessID, msg.Text, defaultTopK)
	if err != nil {
		return Reply{}, fmt.Errorf("faq search: %w", err)
	}

	best := bestScore(matches)
	answered := best >= e.threshold

	answer, err := e.ai.GenerateReply(ctx, msg.Text, matches)
	if err != nil {
		return Reply{}, fmt.Errorf("generate reply: %w", err)
	}

	var matchedFAQID *int64
	if len(matches) > 0 {
		matchedFAQID = &matches[0].FAQID
	}

	reply := Reply{
		Text:         answer,
		Answered:     answered,
		Confidence:   best,
		MatchedFAQID: matchedFAQID,
	}

	e.record(ctx, msg, reply)
	return reply, nil
}

// record logs the conversation and, when the answer was not confident, flags it
// for an admin. Storage failures are logged but never block the customer reply.
func (e *Engine) record(ctx context.Context, msg Message, reply Reply) {
	convID, err := e.store.LogConversation(ctx, ConversationRecord{
		BusinessID:   msg.BusinessID,
		Channel:      msg.Channel,
		UserID:       msg.UserID,
		Question:     msg.Text,
		MatchedFAQID: reply.MatchedFAQID,
		AIAnswer:     reply.Text,
		Confidence:   reply.Confidence,
		WasAnswered:  reply.Answered,
	})
	if err != nil {
		e.log.Error("log conversation failed", "error", err, "business_id", msg.BusinessID)
		return
	}

	if !reply.Answered {
		if err := e.store.EnqueueUnanswered(ctx, convID, msg.Text); err != nil {
			e.log.Error("enqueue unanswered failed", "error", err, "conversation_id", convID)
		}
	}
}

// bestScore returns the top match score, or 0 when there are no matches.
func bestScore(matches []Match) float64 {
	if len(matches) == 0 {
		return 0
	}
	return matches[0].Score
}
