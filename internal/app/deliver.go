package app

import (
	"context"
	"fmt"
	"strconv"

	"github.com/JasonKyawLab/AskDesk/internal/channel/messenger"
	"github.com/JasonKyawLab/AskDesk/internal/channel/telegram"
	"github.com/JasonKyawLab/AskDesk/internal/config"
	"github.com/JasonKyawLab/AskDesk/internal/core"
)

// WebReplyStore queues a reply for a web/widget customer to poll for.
type WebReplyStore interface {
	Add(ctx context.Context, businessID int64, sessionID, message string) error
}

// ChannelDeliverer routes a finished reply to the correct channel: Telegram
// pushes to the chat; web queues the reply for the customer's browser to poll.
// New channels add a case here.
type ChannelDeliverer struct {
	telegram   *telegram.Client
	messenger  *messenger.Client
	web        WebReplyStore
	businessID int64
}

// NewChannelDeliverer builds a deliverer. web may be nil (web delivery disabled).
func NewChannelDeliverer(cfg *config.Config, web WebReplyStore) *ChannelDeliverer {
	var tgOpts []telegram.ClientOption
	if cfg.TelegramAPIURL != "" {
		tgOpts = append(tgOpts, telegram.WithBaseURL(cfg.TelegramAPIURL))
	}
	var msgOpts []messenger.ClientOption
	if cfg.MessengerAPIURL != "" {
		msgOpts = append(msgOpts, messenger.WithBaseURL(cfg.MessengerAPIURL))
	}
	return &ChannelDeliverer{
		telegram:   telegram.NewClient(cfg.TelegramBotToken, tgOpts...),
		messenger:  messenger.NewClient(cfg.MessengerPageToken, msgOpts...),
		web:        web,
		businessID: cfg.BusinessID,
	}
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
	case core.ChannelMessenger:
		return d.messenger.SendMessage(ctx, replyTo, text)
	case core.ChannelWidget:
		if d.web == nil {
			return fmt.Errorf("web delivery is not configured")
		}
		return d.web.Add(ctx, d.businessID, replyTo, text)
	default:
		return fmt.Errorf("no deliverer for channel %q", channel)
	}
}
