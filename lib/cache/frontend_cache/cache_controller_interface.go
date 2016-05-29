package frontend_cache

import (
	"github.com/majestrate/srndv2/lib/model"
	"net/http"
)

// regenerate a newsgroup page
type GroupRegenRequest struct {
	// which newsgroup
	Group string
	// page number
	Page int
}

type CacheController interface {
	RegenAll()
	RegenFrontPage()
	RegenOnModEvent(string, string, string, int)
	RegenerateBoard(group string)
	Regen(msg model.ArticleEntry)

	DeleteThreadMarkup(root_post_id string)
	DeleteBoardMarkup(group string)

	Start()
	Close()

	GetThreadChan() chan model.ArticleEntry
	GetGroupChan() chan GroupRegenRequest

	ServeCached(w http.ResponseWriter, r *http.Request, key string, recache func() string)
}
