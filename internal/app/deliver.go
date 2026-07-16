package app

import (
	"context"
	"fmt"
	"strconv"

	"github.com/JasonKyawLab/AskDesk/internal/channel/telegram"
	"github.com/JasonKyawLab/AskDesk/internal/config"
	"github.com/JasonKyawLab/AskDesk/internal/core"
)

// ChannelDeliverer routes a finished reply to the correct channel client.
// New channels add a case here.
type ChannelDeliverer struct {
	telegram *telegram.Client
}

// NewChannelDeliverer builds a deliverer with a Telegram client from config.
func NewChannelDeliverer(cfg *config.Config) *ChannelDeliverer {
	var opts []telegram.ClientOption
	if cfg.TelegramAPIURL != "" {
		opts = append(opts, telegram.WithBaseURL(cfg.TelegramAPIURL))
	}
	return &ChannelDeliverer{telegram: telegram.NewClient(cfg.TelegramBotToken, opts...)}
}

// Deliver sends text to the reply address on the given channel.
func (d *ChannelDeliverer) Deliver(ctx context.Context, channel core.Channel, replyTo, text string) error {
	switch channel {
	case core.ChannelTelegram:
		chatID, err := strconv.ParseInt(replyTo, 10, 64)
		if err != nil {
			return fmt.Errorf("bad telegram chat id %q: %w", replyTo, err)
		}
		return d.telegram.SendMessage(ctx, chatID, text)
	default:
		return fmt.Errorf("no deliverer for channel %q", channel)
	}
}
