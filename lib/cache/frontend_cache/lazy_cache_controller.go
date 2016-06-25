package frontend_cache

import (
	log "github.com/Sirupsen/logrus"
	"github.com/majestrate/srndv2/lib/cache"
	"github.com/majestrate/srndv2/lib/database"
	"github.com/majestrate/srndv2/lib/model"
	"github.com/majestrate/srndv2/lib/util"
	"net/http"
	"sync"
	"time"
)

type LazyCacheController struct {
	database database.DB
	c        cache.CacheInterface
	regen    MarkupGenerator

	webroot_dir string

	regen_threads int

	regenThreadChan chan model.ArticleEntry
	regenGroupChan  chan GroupRegenRequest
	regenCatalogMap map[string]bool

	ukkoTicker         *time.Ticker
	longTermTicker     *time.Ticker
	regenCatalogTicker *time.Ticker

	regenCatalogLock sync.RWMutex
}

func (self *LazyCacheController) DeleteBoardMarkup(group string) {
	pages, _ := self.database.GetPagesPerBoard(group)
	for page := 0; page < pages; page++ {
		self.invalidateBoardPage(group, page)
	}
}

// try to delete root post's page
func (self *LazyCacheController) DeleteThreadMarkup(root_post_id string) {
	var entry model.ArticleEntry
	group, _, err := self.database.GetPageForRootMessage(root_post_id)
	if err == nil {
		entry[0] = root_post_id
		entry[1] = group
		self.invalidateThreadPage(entry)
	}
}

// regen every newsgroup
func (self *LazyCacheController) RegenAll() {
	log.Println("regen all on http frontend")

	// get all groups
	groups, err := self.database.GetAllNewsgroups()
	if err == nil {
		for _, group := range groups {
			// send every thread for this group down the regen thread channel
			go self.database.GetGroupThreads(group, self.regenThreadChan)
			pages, _ := self.database.GetGroupPageCount(group)
			var pg int64
			for pg = 0; pg < pages; pg++ {
				self.regenGroupChan <- GroupRegenRequest{group, int(pg)}
			}
		}
	}
}

func (self *LazyCacheController) pollLongTerm() {
	for {
		<-self.longTermTicker.C
		// regenerate long term stuff
		self.regenLongTerm()
		self.invalidateFrontPage()
	}
}

func (self *LazyCacheController) invalidateBoardPage(group string, pageno int) {
	key := util.GetFilenameForBoardPage(self.webroot_dir, group, pageno, false)
	self.c.DeleteCache(key)
}

func (self *LazyCacheController) invalidateThreadPage(entry model.ArticleEntry) {
	key := util.GetFilenameForThread(self.webroot_dir, entry.MessageID().String(), false)
	self.c.DeleteCache(key)
	self.invalidateFrontPage()
}

func (self *LazyCacheController) invalidateUkko() {
	key := util.GetFilenameForUkko(self.webroot_dir)
	self.c.DeleteCache(key)
}

func (self *LazyCacheController) invalidateFrontPage() {
	key := util.GetFilenameForIndex(self.webroot_dir)
	self.c.DeleteCache(key)

	key = util.GetFilenameForBoards(self.webroot_dir)
	self.c.DeleteCache(key)
}

func (self *LazyCacheController) invalidateCatalog(group string) {
	key := util.GetFilenameForCatalog(self.webroot_dir, group)
	self.c.DeleteCache(key)
}

func (self *LazyCacheController) pollRegen() {
	for {
		select {
		// listen for regen board requests
		case req := <-self.regenGroupChan:
			self.invalidateBoardPage(req.Group, req.Page)

			self.regenCatalogLock.Lock()
			self.regenCatalogMap[req.Group] = true
			self.regenCatalogLock.Unlock()

			// listen for regen thread requests
		case entry := <-self.regenThreadChan:
			self.invalidateThreadPage(entry)
			// regen ukko
		case _ = <-self.ukkoTicker.C:
			self.invalidateUkko()
		case _ = <-self.regenCatalogTicker.C:
			self.regenCatalogLock.Lock()
			for board, _ := range self.regenCatalogMap {
				self.invalidateCatalog(board)
			}
			self.regenCatalogMap = make(map[string]bool)
			self.regenCatalogLock.Unlock()
		}
	}
}

// regen every page of the board
func (self *LazyCacheController) RegenerateBoard(group string) {
	pages, _ := self.database.GetPagesPerBoard(group)
	for page := 0; page < pages; page++ {
		self.regenerateBoardPage(group, page)
	}
}

// regenerate the catalog for a board
func (self *LazyCacheController) regenerateCatalog(board string) {
	key := util.GetFilenameForCatalog(self.webroot_dir, board)
	self.c.Cache(key, self.regen.GenerateCatalog(board))
}

// regenerate just a thread page
func (self *LazyCacheController) regenerateThread(root model.ArticleEntry) {
	key := util.GetFilenameForThread(self.webroot_dir, root.MessageID().String(), false)
	self.c.Cache(key, self.regen.GenerateThread(root))
}

// regenerate just a page on a board
func (self *LazyCacheController) regenerateBoardPage(board string, page int) {
	key := util.GetFilenameForBoardPage(self.webroot_dir, board, page, false)
	self.c.Cache(key, self.regen.GenerateBoardPage(board, page))
}

// regenerate the front page
func (self *LazyCacheController) RegenFrontPage() {
	key := util.GetFilenameForIndex(self.webroot_dir)
	self.c.Cache(key, self.regen.GenerateFrontPage())

	key = util.GetFilenameForBoards(self.webroot_dir)
	self.c.Cache(key, self.regen.GenerateBoards())
}

func (self *LazyCacheController) regenLongTerm() {
	key := util.GetFilenameForHistory(self.webroot_dir)
	self.c.Cache(key, self.regen.GenerateHistory())
}

// regenerate pages after a mod event
func (self *LazyCacheController) RegenOnModEvent(newsgroup, msgid, root string, page int) {
	if root == msgid {
		self.DeleteThreadMarkup(root)
	} else {
		self.invalidateThreadPage(model.ArticleEntry{root, newsgroup})
	}
	self.regenGroupChan <- GroupRegenRequest{newsgroup, int(page)}
}

func (self *LazyCacheController) Start() {
	threads := self.regen_threads

	// check for invalid number of threads
	if threads <= 0 {
		threads = 1
	}

	// use N threads for regeneration
	for threads > 0 {
		go self.pollRegen()
		threads--
	}
	// run long term regen jobs
	go self.pollLongTerm()
}

func (self *LazyCacheController) Regen(msg model.ArticleEntry) {
	self.regenThreadChan <- msg
	self.RegenerateBoard(msg.Newsgroup().String())
}

func (self *LazyCacheController) GetThreadChan() chan model.ArticleEntry {
	return self.regenThreadChan
}

func (self *LazyCacheController) GetGroupChan() chan GroupRegenRequest {
	return self.regenGroupChan
}

func (self *LazyCacheController) Close() {
	self.c.Close()
}

func (self *LazyCacheController) ServeCached(w http.ResponseWriter, r *http.Request, key string, handler func() string) {
	self.c.ServeCached(w, r, key, handler)
}

func NewLazyCacheController(webroot string, threads int, db database.DB, c cache.CacheInterface, regen MarkupGenerator) CacheController {
	ctl := new(LazyCacheController)

	ctl.longTermTicker = time.NewTicker(time.Hour)
	ctl.ukkoTicker = time.NewTicker(time.Second * 10)
	ctl.regenCatalogTicker = time.NewTicker(time.Second * 20)

	ctl.regenCatalogMap = make(map[string]bool)
	ctl.regenThreadChan = make(chan model.ArticleEntry, 16)
	ctl.regenGroupChan = make(chan GroupRegenRequest, 8)

	ctl.webroot_dir = webroot
	ctl.regen_threads = threads
	ctl.database = db
	ctl.c = c
	ctl.regen = regen

	return ctl
}
