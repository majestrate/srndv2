package srnd

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
)

type NullCache struct {
	database Database
	store    ArticleStore

	webroot_dir string
	name        string

	regen_threads int
	attachments   bool

	prefix          string
	regenThreadChan chan ArticleEntry
	regenGroupChan  chan groupRegenRequest
}

type nullHandler struct {
	cache *NullCache
}

func (self *nullHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, file := filepath.Split(r.URL.Path)
	if len(file) == 0 || file == "index.html" {
		template.genFrontPage(10, self.cache.prefix, self.cache.name, w, ioutil.Discard, self.cache.database)
		return
	}

	isjson := strings.HasSuffix(file, ".json")

	if file == "index.json" {
		// TODO: index.json
		goto notfound
	}
	if strings.HasPrefix(file, "history.html") {
		template.genGraphs(self.cache.prefix, w, self.cache.database)
		return
	}
	if strings.HasPrefix(file, "boards.html") {
		template.genFrontPage(10, self.cache.prefix, self.cache.name, ioutil.Discard, w, self.cache.database)
		return
	}

	if strings.HasPrefix(file, "boards.json") {
		b := self.cache.database.GetAllNewsgroups()
		json.NewEncoder(w).Encode(b)
		return
	}

	if strings.HasPrefix(file, "ukko.html") {
		template.genUkko(self.cache.prefix, self.cache.name, w, self.cache.database, false)
		return
	}
	if strings.HasPrefix(file, "ukko.json") {
		template.genUkko(self.cache.prefix, self.cache.name, w, self.cache.database, true)
		return
	}

	if strings.HasPrefix(file, "ukko-") {
		page := getUkkoPage(file)
		template.genUkkoPaginated(self.cache.prefix, self.cache.name, w, self.cache.database, page, isjson)
		return
	}
	if strings.HasPrefix(file, "thread-") {
		hash := getThreadHash(file)
		if len(hash) == 0 {
			goto notfound
		}
		msg, err := self.cache.database.GetMessageIDByHash(hash)
		if err != nil {
			goto notfound
		}
		template.genThread(self.cache.attachments, msg, self.cache.prefix, self.cache.name, w, self.cache.database, isjson)
		return
	}
	if strings.HasPrefix(file, "catalog-") {
		group := getGroupForCatalog(file)
		if len(group) == 0 {
			goto notfound
		}
		hasgroup := self.cache.database.HasNewsgroup(group)
		if !hasgroup {
			goto notfound
		}
		template.genCatalog(self.cache.prefix, self.cache.name, group, w, self.cache.database)
		return
	} else {
		group, page := getGroupAndPage(file)
		if len(group) == 0 || page < 0 {
			goto notfound
		}
		hasgroup := self.cache.database.HasNewsgroup(group)
		if !hasgroup {
			goto notfound
		}
		pages := self.cache.database.GetGroupPageCount(group)
		if page >= int(pages) {
			goto notfound
		}
		template.genBoardPage(self.cache.attachments, self.cache.prefix, self.cache.name, group, page, w, self.cache.database, isjson)
		return
	}

notfound:
	template.renderNotFound(w, r, self.cache.prefix, self.cache.name)
}

func (self *NullCache) DeleteBoardMarkup(group string) {
}

// try to delete root post's page
func (self *NullCache) DeleteThreadMarkup(root_post_id string) {
}

// regen every newsgroup
func (self *NullCache) RegenAll() {
	// we will do this as it's used by rengen on start for frontend
	groups := self.database.GetAllNewsgroups()
	for _, group := range groups {
		self.database.GetGroupThreads(group, self.regenThreadChan)
	}
}

func (self *NullCache) RegenFrontPage() {
}

func (self *NullCache) pollRegen() {
	for {
		select {
		// consume regen requests
		case _ = <-self.regenGroupChan:
			{
			}
		case _ = <-self.regenThreadChan:
			{
			}
		}
	}
}

// regen every page of the board
func (self *NullCache) RegenerateBoard(group string) {
}

// regenerate pages after a mod event
func (self *NullCache) RegenOnModEvent(newsgroup, msgid, root string, page int) {
}

func (self *NullCache) Start() {
	go self.pollRegen()
}

func (self *NullCache) Regen(msg ArticleEntry) {
}

func (self *NullCache) GetThreadChan() chan ArticleEntry {
	return self.regenThreadChan
}

func (self *NullCache) GetGroupChan() chan groupRegenRequest {
	return self.regenGroupChan
}

func (self *NullCache) GetHandler() http.Handler {
	return &nullHandler{self}
}

func (self *NullCache) Close() {
	//nothig to do
}

func NewNullCache(prefix, webroot, name string, attachments bool, db Database, store ArticleStore) CacheInterface {
	cache := new(NullCache)
	cache.regenThreadChan = make(chan ArticleEntry, 16)
	cache.regenGroupChan = make(chan groupRegenRequest, 8)

	cache.prefix = prefix
	cache.webroot_dir = webroot
	cache.name = name
	cache.attachments = attachments
	cache.database = db
	cache.store = store

	return cache
}
