package frontend

import (
	"github.com/majestrate/srndv2/lib/model"
)

// a frontend that displays nntp posts and allows posting
type Frontend interface {

	// channel that is for the frontend to pool for new posts from the nntpd
	PostsChan() chan Post

	// run mainloop
	Mainloop()

	// do we want posts from a newsgroup?
	AllowNewsgroup(group string) bool

	// trigger a manual regen of indexes for a root post
	Regen(msg model.ArticleEntry)
}
