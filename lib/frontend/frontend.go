package frontend

import (
	"github.com/majestrate/srndv2/lib/model"
)

// a frontend that displays nntp posts and allows posting
type Frontend interface {

	// channel that is for the frontend to pool for new posts from the nntpd
	// nntp -> frontend
	InboundPosts() chan model.PostReference

	// channel that is for the nntp server to poll for new posts from the frontend
	// frontend -> nntp
	OutboundPosts() chan model.PostReference

	// run mainloop
	Mainloop()

	// do we accept this inbound post?
	AllowPost(p model.PostReference) bool

	// trigger a manual regen of indexes for a root post
	Regen(p model.PostReference)
}

type FrontendEventHooks struct {
	Frontend Frontend
}

func (ev *FrontendEventHooks) GotArticle(msgid model.MessageID, group model.Newsgroup) {

}
