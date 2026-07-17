// Package core is the channel-agnostic reply engine: the AI/RAG logic lives here
// once, and every channel adapter funnels normalized messages into it.
package core

// Channel identifies where a message came from. Adapters translate their
// native format into these values so the engine never knows channel specifics.
type Channel string

const (
	ChannelTelegram  Channel = "telegram"
	ChannelWhatsApp  Channel = "whatsapp"
	ChannelMessenger Channel = "messenger"
	ChannelWidget    Channel = "widget"
)

// Message is a normalized inbound customer message. Every channel produces this
// same shape: { businessID, channel, userID, text }.
type Message struct {
	BusinessID int64
	Channel    Channel
	UserID     string // the user's id within their channel (e.g. Telegram user_id)
	UserName   string // display name, for admin visibility (may be empty)
	Text       string
}

// Reply is the engine's response to a Message.
type Reply struct {
	Text         string
	Answered     bool    // true when a confident answer was found
	Confidence   float64 // best FAQ match score, 0..1
	MatchedFAQID *int64  // the FAQ the answer was grounded in, if any
}

// Match is a FAQ returned by similarity search, ordered by descending Score.
type Match struct {
	FAQID    int64
	Question string
	Answer   string
	Score    float64 // cosine similarity, 0..1
}
