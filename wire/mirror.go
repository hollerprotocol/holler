package wire

import "encoding/json"

// Conversation sharing is an extension: an agent may mirror its threads to
// hosts it names (typically a dashboard), so that they can show those
// conversations, not only their subjects and states.
//
// The sharing agent announces it: Hello.Shares and Presence.Shares list the
// hosts it shares with, so the other party of every thread knows. That party
// can keep a thread out with a private message; the sharer then stops
// mirroring it and asks its hosts to forget what they have of it.
const (
	// TMirror carries a copy of one line of a thread the sender is part
	// of. It is reliable like msg: queued, acked by id, replayed on resume
	// (its th is the mirrored thread's, for the seen map).
	TMirror = "mirror"
	// TPrivate asks the receiver not to mirror the thread th. It is
	// reliable like msg.
	TPrivate = "private"
)

// Mirror is one mirrored line, or, with Withdraw, a request to forget a
// mirrored thread.
type Mirror struct {
	Envelope
	// Of is the other party of the mirrored thread (the sender is one side).
	Of      string `json:"of"`
	OfName  string `json:"of_name,omitempty"`
	Subject string `json:"subject,omitempty"`
	// Dir says who wrote Line: "out" the sender, "in" the other party.
	Dir string `json:"dir,omitempty"`
	// Line is the original msg or state line, verbatim. A msg's blob parts
	// keep their name, type and size; the file itself is not mirrored.
	Line json.RawMessage `json:"line,omitempty"`
	// Withdraw asks the receiver to delete everything it has of the thread
	// th between the sender and Of.
	Withdraw bool `json:"withdraw,omitempty"`
}

// Private asks the receiver to keep the thread th out of what it shares.
type Private struct {
	Envelope
}
