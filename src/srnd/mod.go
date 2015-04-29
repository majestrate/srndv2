//
// mod.go
//
package srnd

import (
  "time"
  "fmt"
)

// moderation event
type ModEvent struct {
  Action string
  MessageID string
}

// moderation message
type ModMessage struct {
  Date time.Time
  Events []ModEvent
}

func (self *ModEvent) String() string {
  return fmt.Sprintf("%s %s", self.Action, self.MessageID)
}

func (self *ModMessage) String() string {
  body := "Content-Type: text/plain; charset=UTF-8\n"
  body += fmt.Sprintf("Date: %s\n", self.Date.Format(time.RFC1123Z))
  for idx := range(self.Events) {
    body += fmt.Sprintf("%s\n", self.Events[idx].String())
  }
  return body
}
