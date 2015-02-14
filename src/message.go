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
	Newsgrops string
	From string
	Subject string
	PubKey string
	Signature string
	Posted time.Time
	Message string
	Path string
	Sage bool
}
