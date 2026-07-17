package telegram

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/JasonKyawLab/AskDesk/internal/core"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

type fakeAdminStore struct {
	admin        bool
	pending      []store.PendingQuestion
	target       store.UnansweredTarget
	getErr       error
	replyID      int64
	replySet     bool
	resolved     bool
	cleared      bool
	pendingReply int64
	hasPending   bool
}

func (f *fakeAdminStore) IsAdmin(context.Context, int64, core.Channel, string) (bool, error) {
	return f.admin, nil
}
func (f *fakeAdminStore) TodayStats(context.Context, int64) (store.DailyStats, error) {
	return store.DailyStats{Total: 5, Answered: 3, Unanswered: 2}, nil
}
func (f *fakeAdminStore) PendingUnanswered(context.Context, int64, int) ([]store.PendingQuestion, error) {
	return f.pending, nil
}
func (f *fakeAdminStore) GetUnanswered(context.Context, int64, int64) (store.UnansweredTarget, error) {
	return f.target, f.getErr
}
func (f *fakeAdminStore) ResolveUnanswered(context.Context, int64, int64) error {
	f.resolved = true
	return nil
}
func (f *fakeAdminStore) SetPendingReply(_ context.Context, _ int64, _ core.Channel, _ string, id int64) error {
	f.replySet, f.replyID = true, id
	return nil
}
func (f *fakeAdminStore) ClearPendingReply(context.Context, int64, core.Channel, string) error {
	f.cleared = true
	return nil
}
func (f *fakeAdminStore) PendingReply(context.Context, int64, core.Channel, string) (int64, bool, error) {
	return f.pendingReply, f.hasPending, nil
}

type fakePanelClient struct {
	fakeMenuClient
	sentTo   []int64
	sentMsgs []string
}

func (f *fakePanelClient) SendMessage(_ context.Context, chatID int64, text string) error {
	f.sentTo = append(f.sentTo, chatID)
	f.sentMsgs = append(f.sentMsgs, text)
	return nil
}

func panelHandler(sub *fakeSubmitter, s *fakeAdminStore, c *fakePanelClient) *Handler {
	panel := NewAdminPanel(s, c, nil, "", 1, discardLogger())
	return NewHandler(sub, nil, nil, panel, nil, 1, "", discardLogger())
}

func TestPanel_AdminCommandShowsPanel(t *testing.T) {
	sub := &fakeSubmitter{}
	client := &fakePanelClient{}
	h := panelHandler(sub, &fakeAdminStore{admin: true}, client)

	rec := post(h, `{"message":{"text":"/admin","chat":{"id":5},"from":{"id":9}}}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if sub.called {
		t.Error("/admin from an admin must not reach the submitter")
	}
	if !strings.Contains(client.sentText, "Admin panel") {
		t.Errorf("expected panel, got %q", client.sentText)
	}
}

func TestPanel_NonAdminFallsThrough(t *testing.T) {
	sub := &fakeSubmitter{}
	client := &fakePanelClient{}
	h := panelHandler(sub, &fakeAdminStore{admin: false}, client)

	post(h, `{"message":{"text":"/admin","chat":{"id":5},"from":{"id":9}}}`)

	if !sub.called {
		t.Error("non-admin /admin should fall through to the normal flow")
	}
	if client.sentText != "" {
		t.Error("non-admin must not see the panel")
	}
}

func TestPanel_TapReplyThenTypedMessageRelaysToCustomer(t *testing.T) {
	sub := &fakeSubmitter{}
	s := &fakeAdminStore{admin: true, target: store.UnansweredTarget{
		Channel: core.ChannelTelegram, ReplyTo: "777", UserName: "Aung", Question: "do you deliver?",
	}}
	client := &fakePanelClient{}
	h := panelHandler(sub, s, client)

	// Tap ✍️ Reply on item #4.
	post(h, `{"callback_query":{"id":"cb1","data":"a:r:4","from":{"id":9},"message":{"message_id":7,"chat":{"id":5}}}}`)
	if !s.replySet || s.replyID != 4 {
		t.Fatalf("reply state not set: %+v", s)
	}
	if !strings.Contains(client.editedText, "Type your reply") {
		t.Errorf("expected reply prompt, got %q", client.editedText)
	}

	// Admin types the answer; it must go to the customer, not the AI.
	s.pendingReply, s.hasPending = 4, true
	post(h, `{"message":{"text":"Yes, weekdays 9-6","chat":{"id":5},"from":{"id":9}}}`)

	if sub.called {
		t.Error("reply text must not reach the AI/submitter")
	}
	if len(client.sentTo) < 2 || client.sentTo[0] != 777 || client.sentMsgs[0] != "Yes, weekdays 9-6" {
		t.Fatalf("reply not relayed to customer: to=%v msgs=%v", client.sentTo, client.sentMsgs)
	}
	if !strings.Contains(client.sentMsgs[1], "Sent to Aung") {
		t.Errorf("expected confirmation, got %q", client.sentMsgs[1])
	}
	if !s.resolved || !s.cleared {
		t.Errorf("item should be resolved and state cleared: %+v", s)
	}
}

func TestPanel_ForgedCallbackFromNonAdminIgnored(t *testing.T) {
	sub := &fakeSubmitter{}
	s := &fakeAdminStore{admin: false}
	client := &fakePanelClient{}
	h := panelHandler(sub, s, client)

	post(h, `{"callback_query":{"id":"cb1","data":"a:r:4","from":{"id":666},"message":{"message_id":7,"chat":{"id":5}}}}`)

	if s.replySet {
		t.Error("non-admin callback must not set reply state")
	}
	if client.editedText != "" {
		t.Error("non-admin must not see panel screens")
	}
}

func TestPanel_AdminGreetingShowsPanelButton(t *testing.T) {
	sub := &fakeSubmitter{}
	client := &fakePanelClient{}
	panel := NewAdminPanel(&fakeAdminStore{admin: true}, client, nil, "", 1, discardLogger())
	h := NewHandler(sub, fakeMenuStore{}, client, panel, nil, 1, "", discardLogger())

	post(h, `{"message":{"text":"hi","chat":{"id":5},"from":{"id":9}}}`)

	last := client.sentKb[len(client.sentKb)-1]
	if last[0].Data != cbPanel {
		t.Errorf("admin greeting should end with the panel button, got %+v", last)
	}
}

func TestPanel_CustomerGreetingHasNoPanelButton(t *testing.T) {
	sub := &fakeSubmitter{}
	client := &fakePanelClient{}
	panel := NewAdminPanel(&fakeAdminStore{admin: false}, client, nil, "", 1, discardLogger())
	h := NewHandler(sub, fakeMenuStore{}, client, panel, nil, 1, "", discardLogger())

	post(h, `{"message":{"text":"hi","chat":{"id":5},"from":{"id":9}}}`)

	for _, row := range client.sentKb {
		for _, b := range row {
			if b.Data == cbPanel {
				t.Fatal("customer must not see the admin panel button")
			}
		}
	}
}

func TestPanel_CustomerTextUnaffected(t *testing.T) {
	sub := &fakeSubmitter{}
	s := &fakeAdminStore{admin: false} // customer, no pending state
	client := &fakePanelClient{}
	h := panelHandler(sub, s, client)

	post(h, `{"message":{"text":"do you deliver?","chat":{"id":5},"from":{"id":9}}}`)

	if !sub.called {
		t.Error("customer text should flow to the submitter as usual")
	}
}
