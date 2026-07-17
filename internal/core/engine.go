package core

import (
	"context"
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

// FallbackProvider returns the customer-facing message to send when the AI is
// unavailable. It is per-business and looked up fresh, so it can be edited at
// runtime. Implementations must never fail — return a sensible default.
type FallbackProvider interface {
	Fallback(ctx context.Context, businessID int64) string
}

// ConversationRecord is a single logged interaction (maps to the conversations table).
type ConversationRecord struct {
	BusinessID   int64
	Channel      Channel
	UserID       string
	UserName     string
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
	fallback  FallbackProvider
	log       *slog.Logger
	threshold float64
}

// NewEngine constructs an Engine.
func NewEngine(r Retriever, ai AIProvider, store ConversationStore, fallback FallbackProvider, log *slog.Logger) *Engine {
	return &Engine{
		retriever: r,
		ai:        ai,
		store:     store,
		fallback:  fallback,
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
		// Retrieval (embedding) is down: degrade gracefully instead of going silent.
		e.log.Error("faq search failed; degrading", "error", err, "business_id", msg.BusinessID)
		return e.degrade(ctx, msg), nil
	}

	best := bestScore(matches)
	answered := best >= e.threshold

	answer, err := e.ai.GenerateReply(ctx, msg.Text, matches)
	if err != nil {
		// Every AI provider failed (e.g. quota): send the fallback and flag it.
		e.log.Error("generate reply failed; degrading", "error", err, "business_id", msg.BusinessID)
		return e.degrade(ctx, msg), nil
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

	e.record(ctx, msg, reply.Text, reply.MatchedFAQID, reply.Confidence, reply.Answered)
	return reply, nil
}

// degrade records the question as unanswered (so an admin sees it) and returns
// the per-business fallback message. Used when a provider is unavailable.
func (e *Engine) degrade(ctx context.Context, msg Message) Reply {
	e.record(ctx, msg, "", nil, 0, false)
	return Reply{Text: e.fallback.Fallback(ctx, msg.BusinessID), Answered: false}
}

// record logs the conversation and, when not answered, flags it for an admin.
// Storage failures are logged but never block the customer reply.
func (e *Engine) record(ctx context.Context, msg Message, aiAnswer string, matchedFAQID *int64, confidence float64, answered bool) {
	convID, err := e.store.LogConversation(ctx, ConversationRecord{
		BusinessID:   msg.BusinessID,
		Channel:      msg.Channel,
		UserID:       msg.UserID,
		UserName:     msg.UserName,
		Question:     msg.Text,
		MatchedFAQID: matchedFAQID,
		AIAnswer:     aiAnswer,
		Confidence:   confidence,
		WasAnswered:  answered,
	})
	if err != nil {
		e.log.Error("log conversation failed", "error", err, "business_id", msg.BusinessID)
		return
	}

	if !answered {
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
