package srnd

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
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
	if len(file) == 0 || strings.HasPrefix(file, "index") {
		template.genFrontPage(10, self.cache.prefix, self.cache.name, w, ioutil.Discard, self.cache.database)
		return
	}
	if strings.HasPrefix(file, "history.html") {
		template.genGraphs(self.cache.prefix, w, self.cache.database)
		return
	}
	if strings.HasPrefix(file, "boards.html") {
		template.genFrontPage(10, self.cache.prefix, self.cache.name, ioutil.Discard, w, self.cache.database)
		return
	}
	if strings.HasPrefix(file, "ukko.html") {
		template.genUkko(self.cache.prefix, self.cache.name, w, self.cache.database)
		return
	}
	if strings.HasPrefix(file, "thread-") {
		hash := getThreadHash(file)
		if len(hash) == 0 {
			goto notfound
		}
		msg, err := self.cache.database.GetMessageIDByHash(hash)
		if err != nil {
			fmt.Println("couldn't serve", file, err)
			goto notfound
		}
		template.genThread(self.cache.attachments, msg, self.cache.prefix, self.cache.name, w, self.cache.database)
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
		template.genBoardPage(self.cache.attachments, self.cache.prefix, self.cache.name, group, page, w, self.cache.database)
		return
	}

notfound:
	http.NotFound(w, r)
}

func getThreadHash(file string) (thread string) {
	exp := regexp.MustCompilePOSIX(`thread-([0-9a-f]+)\.html.*`)
	matches := exp.FindStringSubmatch(file)
	if len(matches) != 2 {
		return ""
	}
	thread = matches[1]
	return
}

func getGroupAndPage(file string) (board string, page int) {
	exp := regexp.MustCompilePOSIX(`(.*)-([0-9]+)\.html.*`)
	matches := exp.FindStringSubmatch(file)
	if len(matches) != 3 {
		return "", -1
	}
	var err error
	board = matches[1]
	tmp := matches[2]
	page, err = strconv.Atoi(tmp)
	if err != nil {
		page = -1
	}
	return
}

func (self *NullCache) DeleteBoardMarkup(group string) {
}

// try to delete root post's page
func (self *NullCache) DeleteThreadMarkup(root_post_id string) {
}

// regen every newsgroup
func (self *NullCache) RegenAll() {
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
