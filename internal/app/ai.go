// Package app holds wiring shared by the web and worker binaries.
package app

import (
	"log/slog"

	"github.com/JasonKyawLab/AskDesk/internal/ai"
	"github.com/JasonKyawLab/AskDesk/internal/config"
	"github.com/JasonKyawLab/AskDesk/internal/core"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

// BuildAI selects the generation provider and embedder. With a Gemini key it
// uses Gemini for both (behind the failover chain); without one it falls back
// to a static provider and a dev embedder so the bot still runs end-to-end.
func BuildAI(cfg *config.Config, log *slog.Logger) (core.AIProvider, store.Embedder) {
	if cfg.GeminiAPIKey != "" {
		g := ai.NewGemini(cfg.GeminiAPIKey, ai.WithModels(cfg.GeminiGenModel, cfg.GeminiEmbedModel))
		log.Info("AI: gemini provider enabled")
		return ai.NewChain(log, g), g
	}
	log.Warn("AI: no ASKDESK_GEMINI_API_KEY; using static provider + dev embedder")
	static := ai.StaticProvider{Answer: "Thanks for your message! Our team will get back to you shortly."}
	return ai.NewChain(log, static), ai.HashEmbedder{Dim: 768}
}
