// +build !disable_redis

package srnd

import (
	"bytes"
	"gopkg.in/redis.v3"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Constants for redis key prefixes
// since redis might be shared among many programs, these are used to avoid conflicts.

const CACHE_PREFIX = "NNTPCache::"

//keys - these store the actual data
// for expample NNTPCache::Thread::1234 stores the data of the thread with primary key (hash) 1234

const (
	HISTORY            = CACHE_PREFIX + "History"
	INDEX              = CACHE_PREFIX + "Index"
	BOARDS             = CACHE_PREFIX + "Boards"
	UKKO               = CACHE_PREFIX + "Ukko"
	JSON_UKKO          = "JSON::" + UKKO
	THREAD_PREFIX      = CACHE_PREFIX + "Thread::"
	JSON_THREAD_PREFIX = "JSON::" + THREAD_PREFIX
	GROUP_PREFIX       = CACHE_PREFIX + "Group::"
	JSON_GROUP_PREFIX  = "JSON::" + GROUP_PREFIX
	CATALOG_PREFIX     = CACHE_PREFIX + "Catalog::"
)

type RedisCache struct {
	database Database
	store    ArticleStore
	client   *redis.Client

	webroot_dir string
	name        string

	regen_threads int
	attachments   bool

	prefix          string
	regenThreadChan chan ArticleEntry
	regenGroupChan  chan groupRegenRequest
	regenCatalogMap map[string]bool

	ukkoTicker         *time.Ticker
	longTermTicker     *time.Ticker
	regenCatalogTicker *time.Ticker

	regenCatalogLock sync.RWMutex
}

type redisHandler struct {
	cache *RedisCache
}
type recacheRedis func(io.Writer)

func (self *redisHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, file := filepath.Split(r.URL.Path)
	if len(file) == 0 || strings.HasPrefix(file, "index") {
		self.serveCached(w, r, INDEX, func(out io.Writer) {
			self.cache.regenFrontPageLocal(out, ioutil.Discard)
		})
		return
	}
	if strings.HasPrefix(file, "history.html") {
		self.serveCached(w, r, HISTORY, self.cache.regenLongTerm)
		return
	}
	if strings.HasPrefix(file, "boards.html") {
		self.serveCached(w, r, BOARDS, func(out io.Writer) {
			self.cache.regenFrontPageLocal(ioutil.Discard, out)
		})
		return
	}
	if strings.HasPrefix(file, "ukko.html") {
		self.serveCached(w, r, UKKO, self.cache.regenUkkoMarkup)
		return
	}
	if strings.HasPrefix(file, "ukko.json") {
		self.serveCached(w, r, JSON_UKKO, self.cache.regenUkkoJSON)
		return
	}
	json := strings.HasSuffix(file, ".json")

	if strings.HasPrefix(file, "thread-") {
		hash := getThreadHash(file)
		if len(hash) == 0 {
			goto notfound
		}
		msg, err := self.cache.database.GetMessageIDByHash(hash)
		if err != nil {
			goto notfound
		}
		key := HashMessageID(msg.MessageID())
		if json {
			key = JSON_THREAD_PREFIX + key
		} else {
			key = THREAD_PREFIX + key
		}
		self.serveCached(w, r, key, func(out io.Writer) {
			self.cache.regenerateThread(msg, out, json)
		})
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
		key := CATALOG_PREFIX + group
		self.serveCached(w, r, key, func(out io.Writer) {
			self.cache.regenerateCatalog(group, out)
		})
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
		key := group + "::Page::" + strconv.Itoa(page)
		if json {
			key = JSON_GROUP_PREFIX + key
		} else {
			key = GROUP_PREFIX + key
		}
		self.serveCached(w, r, key, func(out io.Writer) {
			self.cache.regenerateBoardPage(group, page, out, json)
		})
		return
	}

notfound:
	// TODO: cache 404 page?
	template.renderNotFound(w, r, self.cache.prefix, self.cache.name)
}

func (self *redisHandler) serveCached(w http.ResponseWriter, r *http.Request, key string, handler recacheRedis) {
	ts, _ := self.cache.client.Get(key + "::Time").Result()
	var modtime time.Time

	if len(ts) == 0 {
		modtime = time.Now().UTC()
		ts = modtime.Format(http.TimeFormat)
	} else {
		modtime, _ = time.Parse(http.TimeFormat, ts)
	}

	//this is stolen from the Go standard library
	if t, err := time.Parse(http.TimeFormat, r.Header.Get("If-Modified-Since")); err == nil && modtime.Before(t.Add(1*time.Second)) {
		h := w.Header()
		delete(h, "Content-Type")
		delete(h, "Content-Length")
		w.WriteHeader(http.StatusNotModified)
		return
	}

	html, err := self.cache.client.Get(key).Result()

	if err == redis.Nil || len(html) == 0 { //cache miss
		w.Header().Set("Last-Modified", ts)
		handler(w)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Last-Modified", ts)
	io.WriteString(w, html)
}

func (self *RedisCache) DeleteBoardMarkup(group string) {
	pages, _ := self.database.GetPagesPerBoard(group)
	keys := make([]string, 0)
	for page := 0; page < pages; page++ {
		key := GROUP_PREFIX + group + "::Page::" + strconv.Itoa(page)
		keys = append(keys, key, key+"::Time")
		key = JSON_GROUP_PREFIX + group + "::Page::" + strconv.Itoa(page)
		keys = append(keys, key, key+"::Time")
	}
	self.client.Del(keys...)
}

// try to delete root post's page
func (self *RedisCache) DeleteThreadMarkup(root_post_id string) {
	self.client.Del(THREAD_PREFIX + HashMessageID(root_post_id))
	self.client.Del(THREAD_PREFIX + HashMessageID(root_post_id) + "::Time")
	self.client.Del(JSON_THREAD_PREFIX + HashMessageID(root_post_id))
	self.client.Del(JSON_THREAD_PREFIX + HashMessageID(root_post_id) + "::Time")
}

// regen every newsgroup
func (self *RedisCache) RegenAll() {
	log.Println("regen all on http frontend")

	// get all groups
	groups := self.database.GetAllNewsgroups()
	if groups != nil {
		for _, group := range groups {
			// send every thread for this group down the regen thread channel
			go self.database.GetGroupThreads(group, self.regenThreadChan)
			pages := self.database.GetGroupPageCount(group)
			var pg int64
			for pg = 0; pg < pages; pg++ {
				self.regenGroupChan <- groupRegenRequest{group, int(pg)}
			}
		}
	}
}

func (self *RedisCache) regenLongTerm(out io.Writer) {
	buf := new(bytes.Buffer)
	wr := io.MultiWriter(out, buf)
	template.genGraphs(self.prefix, wr, self.database)
	self.cache(HISTORY, buf)
}

func (self *RedisCache) pollLongTerm() {
	for {
		<-self.longTermTicker.C
		// regenerate long term stuff
		self.regenLongTerm(ioutil.Discard)
		self.invalidateFrontPage()
	}
}

func (self *RedisCache) invalidateBoardPage(group string, pageno int) {
	key := group + "::Page::" + strconv.Itoa(pageno)
	self.client.Del(JSON_GROUP_PREFIX+key, GROUP_PREFIX+key)
	self.client.Del(JSON_GROUP_PREFIX+key+"::Time", GROUP_PREFIX+key+"::Time")
}

func (self *RedisCache) invalidateThreadPage(entry ArticleEntry) {
	key := HashMessageID(entry.MessageID())
	self.client.Del(JSON_THREAD_PREFIX+key, THREAD_PREFIX+key)
	self.client.Del(JSON_THREAD_PREFIX+key+"::Time", THREAD_PREFIX+key+"::Time")
	// TODO: do we really want to do this?
	self.invalidateFrontPage()
}

func (self *RedisCache) invalidateUkko() {
	self.client.Del(UKKO, JSON_UKKO, UKKO+"::Time", JSON_UKKO+"::Time")
}

func (self *RedisCache) invalidateFrontPage() {
	self.client.Del(INDEX, INDEX+"::Time", BOARDS, BOARDS+"::Time")
}

func (self *RedisCache) invalidateCatalog(group string) {
	self.client.Del(CATALOG_PREFIX+group, CATALOG_PREFIX+group+"::Time")
}

func (self *RedisCache) pollRegen() {
	for {
		select {
		// listen for regen board requests
		case req := <-self.regenGroupChan:
			self.invalidateBoardPage(req.group, req.page)

			self.regenCatalogLock.Lock()
			self.regenCatalogMap[req.group] = true
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

func (self *RedisCache) cache(key string, buf *bytes.Buffer) {
	tx, err := self.client.Watch(key, key+"::Time")
	defer tx.Close()

	if err != nil {
		log.Println("cannot cache", key, err)
		return
	}
	t := time.Now().UTC()
	ts := t.Format(http.TimeFormat)

	tx.Set(key, buf.String(), 0)
	tx.Set(key+"::Time", ts, 0)

	_, err = tx.Exec(func() error {
		return nil
	})
	if err != nil {
		log.Println("cannot cache", key, err)
	}
}

// regen every page of the board
// TODO do this manually so we can use pipes
func (self *RedisCache) RegenerateBoard(group string) {
	pages := template.prepareGenBoard(self.attachments, self.prefix, self.name, group, self.database)
	for page := 0; page < pages; page++ {
		self.regenerateBoardPage(group, page, ioutil.Discard, false)
		self.regenerateBoardPage(group, page, ioutil.Discard, true)
	}
}

// regenerate the catalog for a board
func (self *RedisCache) regenerateCatalog(board string, out io.Writer) {
	buf := new(bytes.Buffer)
	wr := io.MultiWriter(out, buf)
	template.genCatalog(self.prefix, self.name, board, wr, self.database)
	key := CATALOG_PREFIX + board
	self.cache(key, buf)
}

// regenerate just a thread page
func (self *RedisCache) regenerateThread(root ArticleEntry, out io.Writer, json bool) {
	msgid := root.MessageID()
	buf := new(bytes.Buffer)
	wr := io.MultiWriter(out, buf)

	template.genThread(self.attachments, root, self.prefix, self.name, wr, self.database, json)

	key := HashMessageID(msgid)

	if json {
		key = JSON_THREAD_PREFIX + key
	} else {
		key = THREAD_PREFIX + key
	}

	self.cache(key, buf)
}

// regenerate just a page on a board
func (self *RedisCache) regenerateBoardPage(board string, page int, out io.Writer, json bool) {
	buf := new(bytes.Buffer)
	wr := io.MultiWriter(out, buf)
	template.genBoardPage(self.attachments, self.prefix, self.name, board, page, wr, self.database, json)
	key := board + "::Page::" + strconv.Itoa(page)
	if json {
		key = JSON_GROUP_PREFIX + key
	} else {
		key = GROUP_PREFIX + key
	}
	self.cache(key, buf)
}

// regenerate the front page
func (self *RedisCache) regenFrontPageLocal(indexout, boardsout io.Writer) {
	indexbuf := new(bytes.Buffer)
	indexwr := io.MultiWriter(indexout, indexbuf)
	boardsbuf := new(bytes.Buffer)
	boardswr := io.MultiWriter(boardsout, boardsbuf)

	template.genFrontPage(10, self.prefix, self.name, indexwr, boardswr, self.database)
	self.cache(INDEX, indexbuf)
	self.cache(BOARDS, boardsbuf)
}

func (self *RedisCache) RegenFrontPage() {
	self.regenFrontPageLocal(ioutil.Discard, ioutil.Discard)
}

// regenerate the overboard html
func (self *RedisCache) regenUkkoMarkup(out io.Writer) {
	buf := new(bytes.Buffer)
	wr := io.MultiWriter(out, buf)
	template.genUkko(self.prefix, self.name, wr, self.database, false)
	self.cache(UKKO, buf)
}

// regenerate the overboard json
func (self *RedisCache) regenUkkoJSON(out io.Writer) {
	buf := new(bytes.Buffer)
	wr := io.MultiWriter(out, buf)
	template.genUkko(self.prefix, self.name, wr, self.database, true)
	self.cache(JSON_UKKO, buf)
}

// regenerate pages after a mod event
func (self *RedisCache) RegenOnModEvent(newsgroup, msgid, root string, page int) {
	if root == msgid {
		self.DeleteThreadMarkup(root)
	} else {
		self.invalidateThreadPage(ArticleEntry{root, newsgroup})
	}
	self.regenGroupChan <- groupRegenRequest{newsgroup, int(page)}
}

func (self *RedisCache) Start() {
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

func (self *RedisCache) Regen(msg ArticleEntry) {
	self.regenThreadChan <- msg
	self.RegenerateBoard(msg.Newsgroup())
}

func (self *RedisCache) GetThreadChan() chan ArticleEntry {
	return self.regenThreadChan
}

func (self *RedisCache) GetGroupChan() chan groupRegenRequest {
	return self.regenGroupChan
}

func (self *RedisCache) GetHandler() http.Handler {
	return &redisHandler{self}
}

func (self *RedisCache) Close() {
	if self.client != nil {
		self.client.Close()
		self.client = nil
	}
}

func NewRedisCache(prefix, webroot, name string, threads int, attachments bool, db Database, host, port, password string) CacheInterface {
	cache := new(RedisCache)

	cache.longTermTicker = time.NewTicker(time.Hour)
	cache.ukkoTicker = time.NewTicker(time.Second * 10)
	cache.regenCatalogTicker = time.NewTicker(time.Second * 20)

	cache.regenCatalogMap = make(map[string]bool)
	cache.regenThreadChan = make(chan ArticleEntry, 16)
	cache.regenGroupChan = make(chan groupRegenRequest, 8)

	cache.prefix = prefix
	cache.webroot_dir = webroot
	cache.name = name
	cache.regen_threads = threads
	cache.attachments = attachments
	cache.database = db

	log.Println("Connecting to redis...")

	cache.client = redis.NewClient(&redis.Options{
		Addr:        net.JoinHostPort(host, port),
		Password:    password,
		DB:          0, // use default DB
		PoolTimeout: 10 * time.Second,
		PoolSize:    100,
	})

	_, err := cache.client.Ping().Result() //check for successful connection
	if err != nil {
		log.Fatalf("cannot open connection to redis: %s", err)
	}

	return cache
}
