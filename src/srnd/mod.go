//
// mod.go
// post moderation
//
package srnd

import (
  "fmt"
  "net/http"
  "strings"
  "time"
)


// interface for moderation ui
type ModUI interface {

  // channel for daemon to poll for nntp articles from the mod ui
  MessageChan() chan NNTPMessage

  // check if this key is allowed to access
  // return true if it can otherwise false
  CheckKey(privkey string) (bool, error)

  // serve the base page
  ServeModPage(wr http.ResponseWriter, r *http.Request)
  // handle a login POST request
  HandleLogin(wr http.ResponseWriter, r *http.Request)
  // handle a delete article request 
  HandleDeletePost(wr http.ResponseWriter, r *http.Request)
  // handle a ban address request
  HandleBanAddress(wr http.ResponseWriter, r *http.Request)
  // handle an unban address request
  HandleUnbanAddress(wr http.ResponseWriter, r *http.Request)
  // handle add a pubkey
  HandleAddPubkey(wr http.ResponseWriter, r *http.Request)
  // handle removing a pubkey
  HandleDelPubkey(wr http.ResponseWriter, r *http.Request)
  // handle key generation
  HandleKeyGen(wr http.ResponseWriter, r *http.Request)
}

type ModEvent interface {
  // turn it into a string for putting into an article
  String() string
  // what type of mod event
  Action() string
  // what reason for the event
  Reason() string
  // what is the event acting on
  Target() string
  // scope of the event, regex of newsgroup
  Scope() string
  // when this mod event expires, unix nano
  Expires() int64
}

type simpleModEvent string

func (self simpleModEvent) String() string {
  return string(self)
}

func (self simpleModEvent) Action() string {
  return strings.Split(string(self), " ")[0]
}

func (self simpleModEvent) Reason() string {
  return ""
}

func (self simpleModEvent) Target() string {
  return strings.Split(string(self), " ")[1]
}

func (self simpleModEvent) Scope() string {
  // TODO: hard coded
  return "overchan.*"
}

func (self simpleModEvent) Expires() int64 {
  // no expiration
  return -1
}

// moderation message
type ModMessage struct {
  Date time.Time
  Events []ModEvent
}

// write this mod message out
func (self ModMessage) WriteTo(wr io.Writer, delim []byte) (err error) {
  err = io.WriteString(wr, "Content-Type: text/plain; charset=UTF-8")
  err = wr.Write(delim)
  err = io.WriteString(wr, fmt.Sprintf("Date: %s", self.Date.Format(time.RFC1123Z)))
  err = wr.Write(delim)
  // done headers
  err = wr.Write(delim)
  // write body
  for _, ev := range(self.Events) {
    err = io.WriteString(wr, ev.String())
    err = wr.Write(delim)
  }
  return body
}



func ParseModEvent(line string) ModEvent {
  return simpleModEvent(line)
}
