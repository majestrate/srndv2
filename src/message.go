//
// message.go
//
package main

import (
	"time"
)

type NNTPMessage struct {
	MessageID string
	Reference string
	Newsgroup string
	From string
	Subject string
	PubKey string
	Signature string
	Posted time.Time
	Message string
	Path string
	Sage bool
}
