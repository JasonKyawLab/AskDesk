package admin

import (
	"context"
	"strings"
	"testing"

	"github.com/JasonKyawLab/AskDesk/internal/auth"
	"github.com/JasonKyawLab/AskDesk/internal/core"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

type fakeStore struct {
	admin    bool
	stats    store.DailyStats
	pending  []store.PendingQuestion
	target   store.UnansweredTarget
	getErr   error
	resolved bool
}

func (f *fakeStore) IsAdmin(context.Context, int64, core.Channel, string) (bool, error) {
	return f.admin, nil
}
func (f *fakeStore) TodayStats(context.Context, int64) (store.DailyStats, error) {
	return f.stats, nil
}
func (f *fakeStore) PendingUnanswered(context.Context, int64, int) ([]store.PendingQuestion, error) {
	return f.pending, nil
}
func (f *fakeStore) GetUnanswered(context.Context, int64, int64) (store.UnansweredTarget, error) {
	return f.target, f.getErr
}
func (f *fakeStore) ResolveUnanswered(context.Context, int64, int64) error {
	f.resolved = true
	return nil
}

type fakeDeliverer struct {
	channel core.Channel
	replyTo string
	text    string
	called  bool
}

func (d *fakeDeliverer) Deliver(_ context.Context, ch core.Channel, replyTo, text string) error {
	d.called = true
	d.channel = ch
	d.replyTo = replyTo
	d.text = text
	return nil
}

func TestHandleCommand_NotACommand(t *testing.T) {
	svc := NewService(&fakeStore{admin: true}, nil, nil, "")
	_, handled, err := svc.HandleCommand(context.Background(), 1, core.ChannelTelegram, "u", "hello there")
	if err != nil {
		t.Fatal(err)
	}
	if handled {
		t.Error("plain text should not be handled as a command")
	}
}

func TestHandleCommand_NonAdminNotHandled(t *testing.T) {
	svc := NewService(&fakeStore{admin: false}, nil, nil, "")
	_, handled, err := svc.HandleCommand(context.Background(), 1, core.ChannelTelegram, "u", "/stats")
	if err != nil {
		t.Fatal(err)
	}
	if handled {
		t.Error("non-admin command must fall through, not be handled")
	}
}

func TestHandleCommand_Stats(t *testing.T) {
	svc := NewService(&fakeStore{admin: true, stats: store.DailyStats{Total: 10, Answered: 7, Unanswered: 3}}, nil, nil, "")
	reply, handled, err := svc.HandleCommand(context.Background(), 1, core.ChannelTelegram, "u", "/stats")
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected /stats to be handled")
	}
	for _, want := range []string{"Total: 10", "Answered: 7", "Unanswered: 3"} {
		if !strings.Contains(reply, want) {
			t.Errorf("reply missing %q; got:\n%s", want, reply)
		}
	}
}

func TestHandleCommand_UnansweredEmptyAndList(t *testing.T) {
	empty := NewService(&fakeStore{admin: true}, nil, nil, "")
	reply, handled, _ := empty.HandleCommand(context.Background(), 1, core.ChannelTelegram, "u", "/unanswered")
	if !handled || !strings.Contains(reply, "No pending") {
		t.Errorf("empty case: handled=%v reply=%q", handled, reply)
	}

	withItems := NewService(&fakeStore{admin: true, pending: []store.PendingQuestion{
		{ID: 1, Question: "do you ship overseas?"},
		{ID: 2, Question: "opening hours?"},
	}}, nil, nil, "")
	reply, handled, _ = withItems.HandleCommand(context.Background(), 1, core.ChannelTelegram, "u", "/unanswered")
	if !handled || !strings.Contains(reply, "do you ship overseas?") || !strings.Contains(reply, "opening hours?") {
		t.Errorf("list case: handled=%v reply=%q", handled, reply)
	}
}

func TestHandleCommand_Unknown(t *testing.T) {
	svc := NewService(&fakeStore{admin: true}, nil, nil, "")
	reply, handled, _ := svc.HandleCommand(context.Background(), 1, core.ChannelTelegram, "u", "/bogus")
	if !handled || !strings.Contains(reply, "Commands:") {
		t.Errorf("unknown command: handled=%v reply=%q", handled, reply)
	}
}

func TestHandleCommand_EditLink(t *testing.T) {
	signer := auth.NewSigner("test-secret")
	svc := NewService(&fakeStore{admin: true}, nil, signer, "https://askdesk.example.com")

	reply, handled, err := svc.HandleCommand(context.Background(), 1, core.ChannelTelegram, "999", "/edit")
	if err != nil {
		t.Fatal(err)
	}
	if !handled || !strings.Contains(reply, "https://askdesk.example.com/edit?token=") {
		t.Errorf("edit command: handled=%v reply=%q", handled, reply)
	}
}

func TestHandleCommand_EditDisabledWhenUnconfigured(t *testing.T) {
	svc := NewService(&fakeStore{admin: true}, nil, nil, "")
	reply, handled, _ := svc.HandleCommand(context.Background(), 1, core.ChannelTelegram, "999", "/edit")
	if !handled || !strings.Contains(reply, "not configured") {
		t.Errorf("edit disabled: handled=%v reply=%q", handled, reply)
	}
}

func TestHandleCommand_Reply(t *testing.T) {
	fs := &fakeStore{admin: true, target: store.UnansweredTarget{Channel: core.ChannelTelegram, ReplyTo: "555", Question: "do you deliver?"}}
	del := &fakeDeliverer{}
	svc := NewService(fs, del, nil, "")

	reply, handled, err := svc.HandleCommand(context.Background(), 1, core.ChannelTelegram, "999", "/reply 12 Yes, weekdays 9-6")
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected /reply to be handled")
	}
	if !del.called || del.replyTo != "555" || del.text != "Yes, weekdays 9-6" {
		t.Errorf("reply not delivered correctly: %+v", del)
	}
	if !fs.resolved {
		t.Error("item should be marked resolved after reply")
	}
	if !strings.Contains(reply, "Sent to the customer") {
		t.Errorf("unexpected confirmation: %q", reply)
	}
}

func TestHandleCommand_ReplyUsage(t *testing.T) {
	svc := NewService(&fakeStore{admin: true}, &fakeDeliverer{}, nil, "")
	reply, handled, _ := svc.HandleCommand(context.Background(), 1, core.ChannelTelegram, "999", "/reply")
	if !handled || !strings.Contains(reply, "Usage") {
		t.Errorf("expected usage message, got handled=%v reply=%q", handled, reply)
	}
}
