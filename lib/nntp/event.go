package nntp

import (
	"fmt"
	"strings"
)

// an nntp stream event
// these are pipelined between nntp servers
type StreamEvent string

func (ev StreamEvent) MessageID() MessageID {
	parts := strings.Split(string(ev), " ")
	if len(parts) > 1 {
		return MessageID(parts[1])
	}
	return ""
}

func (ev StreamEvent) String() string {
	return string(ev)
}

func (ev StreamEvent) Command() string {
	return strings.Split(ev.String(), " ")[0]
}

func (ev StreamEvent) Valid() bool {
	return strings.Count(ev.String(), " ") == 1 && ev.MessageID().Valid()
}

func TAKETHIS(msgid MessageID) StreamEvent {
	if msgid.Valid() {
		return StreamEvent(fmt.Sprintf("TAKETHIS %s", msgid))
	} else {
		return ""
	}
}

func CHECK(msgid MessageID) StreamEvent {
	if msgid.Valid() {
		return StreamEvent(fmt.Sprintf("CHECK %s", msgid))
	} else {
		return ""
	}
}

type StreamEventHandler interface {
	HandleStreamEvent(ev StreamEvent, c Conn)
}
