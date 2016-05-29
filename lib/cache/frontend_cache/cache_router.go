package frontend_cache

import (
	"github.com/majestrate/srndv2/lib/database"
	"github.com/majestrate/srndv2/lib/model"
	"github.com/majestrate/srndv2/lib/util"
	"net/http"
	"path/filepath"
	"strings"
)

type CacheRouter struct {
	regen   MarkupGenerator
	ctl     CacheController
	webroot string
	db      database.DB
}

func (self *CacheRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, file := filepath.Split(r.URL.Path)
	key := filepath.Join(self.webroot, file)
	if len(file) == 0 || strings.HasPrefix(file, "index") {
		self.ctl.ServeCached(w, r, filepath.Join(self.webroot, "index.html"), self.regen.GenerateFrontPage)
		return
	}
	if file == "history.html" {
		self.ctl.ServeCached(w, r, key, self.regen.GenerateHistory)
		return
	}
	if file == "boards.html" {
		self.ctl.ServeCached(w, r, key, self.regen.GenerateBoards)
		return
	}
	if file == "ukko.html" {
		self.ctl.ServeCached(w, r, key, self.regen.GenerateUkko)
		return
	}

	if strings.HasPrefix(file, "thread-") {
		hash := util.GetThreadHashHTML(file)
		if len(hash) == 0 {
			goto notfound
		}
		msg, err := self.db.GetMessageIDByHash(hash)
		if err != nil {
			goto notfound
		}

		self.ctl.ServeCached(w, r, key, func() string {
			return self.regen.GenerateThread(msg)
		})
		return
	}
	if strings.HasPrefix(file, "catalog-") {
		group := util.GetGroupForCatalogHTML(file)
		if len(group) == 0 {
			goto notfound
		}
		hasgroup, err := self.db.HasNewsgroup(group)
		if err != nil || !hasgroup {
			goto notfound
		}

		self.ctl.ServeCached(w, r, key, func() string {
			return self.regen.GenerateCatalog(group)
		})
		return
	} else {
		group, page := util.GetGroupAndPageHTML(file)
		if len(group) == 0 || page < 0 {
			goto notfound
		}
		hasgroup, err := self.db.HasNewsgroup(group)
		if err != nil || !hasgroup {
			goto notfound
		}
		pages, err := self.db.GetGroupPageCount(group)
		if err != nil || page >= int(pages) {
			goto notfound
		}
		self.ctl.ServeCached(w, r, key, func() string {
			return self.regen.GenerateBoardPage(group, page)
		})
		return
	}

notfound:
	// TODO: cache 404 page?
	//template.renderNotFound(w, r, self.cache.prefix, self.cache.name)
	http.NotFound(w, r)
}

func (self *CacheRouter) DeleteBoardMarkup(group string) {
	self.ctl.DeleteBoardMarkup(group)
}

// try to delete root post's page
func (self *CacheRouter) DeleteThreadMarkup(root_post_id string) {
	self.ctl.DeleteThreadMarkup(root_post_id)
}

// regen every newsgroup
func (self *CacheRouter) RegenAll() {
	self.ctl.RegenAll()
}

// regen every page of the board
func (self *CacheRouter) RegenerateBoard(group string) {
	self.ctl.RegenerateBoard(group)
}

// regenerate pages after a mod event
func (self *CacheRouter) RegenOnModEvent(newsgroup, msgid, root string, page int) {
	self.ctl.RegenOnModEvent(newsgroup, msgid, root, page)
}

func (self *CacheRouter) Start() {
	self.ctl.Start()
}

func (self *CacheRouter) Close() {
	self.ctl.Close()
}

func (self *CacheRouter) GetThreadChan() chan model.ArticleEntry {
	return self.GetThreadChan()
}

func (self *CacheRouter) GetGroupChan() chan GroupRegenRequest {
	return self.ctl.GetGroupChan()
}

func NewCacheRouter(webroot string, ctl CacheController, regen MarkupGenerator, db database.DB) *CacheRouter {
	router := new(CacheRouter)
	router.regen = regen
	router.db = db
	router.ctl = ctl
	router.webroot = webroot

	return router
}
