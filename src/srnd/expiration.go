//
// expiration.go
// content expiration
//
package srnd

import (
	"log"
	"os"
	"path/filepath"
)

// content expiration interface
type ExpirationCore interface {
	// do expiration for a group
	ExpireGroup(newsgroup string, keep int)
	// Delete a single post and all children
	ExpirePost(messageID string)
	// expire all orphaned articles
	ExpireOrphans()
	// run our mainloop
	Mainloop()
}

func createExpirationCore(database Database, store ArticleStore) ExpirationCore {
	return expire{database, store, make(chan deleteEvent)}
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
	store    ArticleStore
	// channel to send delete requests down
	delChan chan deleteEvent
}

func (self expire) ExpirePost(messageID string) {
	// get article headers
	headers := self.store.GetHeaders(messageID)
	if headers == nil {
		log.Println("failed to load headers for", messageID)
		return
	}
	// is this a root post ?
	ref := headers.Get("References", "")
	if ref == "" {
		// ya, expire the entire thread
		self.ExpireThread(messageID)
	} else {
		// nah, just expire this post
		self.delChan <- deleteEvent(self.store.GetFilename(messageID))
	}
}

func (self expire) ExpireGroup(newsgroup string, keep int) {
	log.Println("Expire group", newsgroup, keep)
	threads := self.database.GetRootPostsForExpiration(newsgroup, keep)
	for _, root := range threads {
		self.ExpireThread(root)
	}
}

func (self expire) ExpireThread(rootMsgid string) {
	replies, err := self.database.GetMessageIDByHeader("References", rootMsgid)
	if err == nil {
		for _, reply := range replies {
			self.delChan <- deleteEvent(self.store.GetFilename(reply))
		}
	}
	self.database.DeleteThread(rootMsgid)
}

// expire all orphaned articles
func (self expire) ExpireOrphans() {
	// get all articles in database
	articles := self.database.GetAllArticles()
	if articles != nil {
		log.Println("expire all orphan posts")
		// for each article
		for _, article := range articles {
			// load headers
			hdr := self.store.GetHeaders(article.MessageID())
			if hdr == nil {
				// article does not exist?
			} else {
				// check if we are a reply
				rootMsgid := hdr.Get("References", "")
				if len(rootMsgid) == 0 {
					// root post
				} else {
					// reply
					// do we have this root post?
					if self.store.HasArticle(rootMsgid) {
						// yes, do nothing
					} else {
						// no, expire post
						self.ExpirePost(article.MessageID())
					}
				}
			}
		}
	}
}

func (self expire) Mainloop() {
	for {
		ev := <-self.delChan
		log.Println("expire", ev.MessageID())
		atts := self.database.GetPostAttachments(ev.MessageID())
		// remove all attachments
		if atts != nil {
			for _, att := range atts {
				img := self.store.AttachmentFilepath(att)
				os.Remove(img)
				thm := self.store.ThumbnailFilepath(att)
				os.Remove(thm)
			}
		}
		err := self.database.BanArticle(ev.MessageID(), "expired")
		if err != nil {
			log.Println("failed to ban for expiration", err)
		}
		err = self.database.DeleteArticle(ev.MessageID())
		if err != nil {
			log.Println("failed to delete article", err)
		}
		// remove article
		os.Remove(ev.Path())
	}
}
