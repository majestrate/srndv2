//
// expiration.go
// content expiration 
//
package srnd

import (
  "path/filepath"
  "log"
  "os"
)

// content expiration interface
type Expiration interface {
  // do expiration for a group
  ExpireGroup(newsgroup string, keep int)
  // Delete a single post and all children
  DeletePost(messageID string)
  // run our mainloop
  Mainloop()
}

type deleteEvent string

func (self deleteEvent) Path() string {
  return string(self)
}

func (self deleteEvent) MessageID() string {
  return filepath.Base(string(self))
}

type expire struct {
  database Database
  store *ArticleStore
  // channel to send delete requests down
  delChan chan deleteEvent
}

func (self expire) DeletePost(messageID string) {
  // get article headers
  nntp  := self.store.GetHeaders(messageID)
  if nntp == nil {
    log.Println("failed to load headers for", messageID)
    return
  }
  // is this a root post ?
  if len(nntp.Reference) == 0 {
    // ya, get all replies
    replies := self.database.GetThreadReplies(nntp.MessageID, 0)
    if replies != nil {
      for _, repl := range replies {
        // scehedule delete of the reply
        self.delChan <- deleteEvent(self.store.GetFilename(repl))
      }
    } else {
      log.Println("failed to get replies for", messageID)
    }
  }
  self.delChan <- deleteEvent(self.store.GetFilename(nntp.MessageID))
}

func (self expire) ExpireGroup(newsgroup string, keep int) {
  threads := self.database.GetRootPostsForExpiration(newsgroup, keep)
  for _, root := range threads {
    self.DeletePost(root)
  }
}

func (self expire) Mainloop() {
  for {
    select {
    case ev := <- self.delChan:
      atts := self.database.GetPostAttachments(ev.MessageID())
      // remove all attachments
      if atts != nil {
        for _, att := range atts {
          img := self.store.AttachmentFilename(att)
          os.Remove(img)
          thm := self.store.ThumbnailFilename(att)
          os.Remove(thm)
        }
      }
      // remove article
      os.Remove(ev.Path())
    }
  }
}
