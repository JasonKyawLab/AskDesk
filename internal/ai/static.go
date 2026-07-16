package ai

import (
	"context"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

// StaticProvider returns a fixed answer. It lets the engine run end-to-end
// locally (and in tests) before a real provider with API keys is configured.
type StaticProvider struct {
	Answer string
}

func (StaticProvider) Name() string { return "static" }

func (p StaticProvider) GenerateReply(context.Context, string, []core.Match) (string, error) {
	return p.Answer, nil
}
