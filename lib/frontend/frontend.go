package frontend

import (
	"github.com/majestrate/srndv2/lib/model"
)

// a frontend that displays nntp posts and allows posting
type Frontend interface {
	// process a new article that was posted
	// this post can either be from the frontend itself or from a remote
	// poster from another frontend
	ProcessArticle(a *model.Article)
}
