package srnd

import (
	"bytes"
	"fmt"
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
	HISTORY       = CACHE_PREFIX + "History"
	INDEX         = CACHE_PREFIX + "Index"
	BOARDS        = CACHE_PREFIX + "Boards"
	UKKO          = CACHE_PREFIX + "Ukko"
	THREAD_PREFIX = CACHE_PREFIX + "Thread::"
	GROUP_PREFIX  = CACHE_PREFIX + "Group::"
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
	regenBoardMap   map[string]groupRegenRequest
	regenThreadMap  map[string]ArticleEntry

	regenBoardTicker  *time.Ticker
	ukkoTicker        *time.Ticker
	longTermTicker    *time.Ticker
	regenThreadTicker *time.Ticker

	regenThreadLock sync.RWMutex
	regenBoardLock  sync.RWMutex
}

type redisHandler struct {
	cache *RedisCache
}

func (self *redisHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, file := filepath.Split(r.URL.Path)
	if len(file) == 0 || strings.HasPrefix(file, "index") {
		self.serveIndex(w, r)
		return
	}
	if strings.HasPrefix(file, "history.html") {
		self.serveHistory(w, r)
		return
	}
	if strings.HasPrefix(file, "boards.html") {
		self.serveBoards(w, r)
		return
	}
	if strings.HasPrefix(file, "ukko.html") {
		self.serveUkko(w, r)
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
		self.serveThread(w, r, msg)
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
		self.serveBoardPage(w, r, group, page)
		return
	}

notfound:
	http.NotFound(w, r)
}

func (self *redisHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	html, err := self.cache.client.Get(INDEX).Result()

	if err == redis.Nil || len(html) == 0 { //cache miss
		self.cache.regenFrontPageLocal(w, ioutil.Discard)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	io.WriteString(w, html)
}

func (self *redisHandler) serveBoards(w http.ResponseWriter, r *http.Request) {
	html, err := self.cache.client.Get(BOARDS).Result()

	if err == redis.Nil || len(html) == 0 { //cache miss
		self.cache.regenFrontPageLocal(ioutil.Discard, w)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	io.WriteString(w, html)
}

func (self *redisHandler) serveHistory(w http.ResponseWriter, r *http.Request) {
	html, err := self.cache.client.Get(HISTORY).Result()

	if err == redis.Nil || len(html) == 0 { //cache miss
		self.cache.regenLongTerm(w)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	io.WriteString(w, html)
}

func (self *redisHandler) serveUkko(w http.ResponseWriter, r *http.Request) {
	html, err := self.cache.client.Get(UKKO).Result()

	if err == redis.Nil || len(html) == 0 { //cache miss
		self.cache.regenUkko(w)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	io.WriteString(w, html)
}

func (self *redisHandler) serveThread(w http.ResponseWriter, r *http.Request, root ArticleEntry) {
	msgid := root.MessageID()
	html, err := self.cache.client.Get(THREAD_PREFIX + HashMessageID(msgid)).Result()

	if err == redis.Nil || len(html) == 0 { //cache miss
		self.cache.regenerateThread(root, w)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	io.WriteString(w, html)
}

func (self *redisHandler) serveBoardPage(w http.ResponseWriter, r *http.Request, board string, page int) {
	html, err := self.cache.client.Get(GROUP_PREFIX + board + "::Page::" + strconv.Itoa(page)).Result()

	if err == redis.Nil || len(html) == 0 { //cache miss
		self.cache.regenerateBoardPage(board, page, w)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	io.WriteString(w, html)
}

func (self *RedisCache) MarkThreadDirty(root ArticleEntry) {
	// we don't care as we are not dynamicly generated
}

func (self *RedisCache) DeleteBoardMarkup(group string) {
	pages, _ := self.database.GetPagesPerBoard(group)
	keys := make([]string, 0)
	for page := 0; page < pages; page++ {
		key := GROUP_PREFIX + group + "::Page::" + strconv.Itoa(page)
		keys = append(keys, key)
	}
	self.client.Del(keys...)
}

// try to delete root post's page
func (self *RedisCache) DeleteThreadMarkup(root_post_id string) {
	self.client.Del(THREAD_PREFIX + HashMessageID(root_post_id))
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
	_, err := self.client.Set(HISTORY, buf.String(), 0).Result()
	if err != nil {
		log.Println("cannot cache history graph", err)
	}
}

func (self *RedisCache) pollLongTerm() {
	for {
		<-self.longTermTicker.C
		// regenerate long term stuff
		self.regenLongTerm(ioutil.Discard)
	}
}

func (self *RedisCache) pollRegen() {
	for {
		select {
		// listen for regen board requests
		case req := <-self.regenGroupChan:
			self.regenBoardLock.Lock()
			self.regenBoardMap[fmt.Sprintf("%s|%s", req.group, req.page)] = req
			self.regenBoardLock.Unlock()
			// listen for regen thread requests
		case entry := <-self.regenThreadChan:
			self.regenThreadLock.Lock()
			self.regenThreadMap[fmt.Sprintf("%s|%s", entry[0], entry[1])] = entry
			self.regenThreadLock.Unlock()
			// regen ukko
		case _ = <-self.ukkoTicker.C:
			self.regenUkko(ioutil.Discard)
			self.regenFrontPageLocal(ioutil.Discard, ioutil.Discard)
		case _ = <-self.regenThreadTicker.C:
			self.regenThreadLock.Lock()
			for _, entry := range self.regenThreadMap {
				self.regenerateThread(entry, ioutil.Discard)
			}
			self.regenThreadMap = make(map[string]ArticleEntry)
			self.regenThreadLock.Unlock()
		case _ = <-self.regenBoardTicker.C:
			self.regenBoardLock.Lock()
			for _, v := range self.regenBoardMap {
				self.regenerateBoardPage(v.group, v.page, ioutil.Discard)
			}
			self.regenBoardMap = make(map[string]groupRegenRequest)
			self.regenBoardLock.Unlock()
		}
	}
}

// regen every page of the board
// TODO do this manually so we can use pipes
func (self *RedisCache) RegenerateBoard(group string) {
	pages := template.prepareGenBoard(self.attachments, self.prefix, self.name, group, self.database)
	for page := 0; page < pages; page++ {
		self.regenerateBoardPage(group, page, ioutil.Discard)
	}
}

// regenerate just a thread page
func (self *RedisCache) regenerateThread(root ArticleEntry, out io.Writer) {
	msgid := root.MessageID()
	buf := new(bytes.Buffer)
	wr := io.MultiWriter(out, buf)
	template.genThread(self.attachments, root, self.prefix, self.name, wr, self.database)
	_, err := self.client.Set(THREAD_PREFIX+HashMessageID(msgid), buf.String(), 0).Result()
	if err != nil {
		log.Println("cannot cache thread", msgid, err)
	}
}

// regenerate just a page on a board
func (self *RedisCache) regenerateBoardPage(board string, page int, out io.Writer) {
	buf := new(bytes.Buffer)
	wr := io.MultiWriter(out, buf)
	template.genBoardPage(self.attachments, self.prefix, self.name, board, page, wr, self.database)
	_, err := self.client.Set(GROUP_PREFIX+board+"::Page::"+strconv.Itoa(page), buf.String(), 0).Result()
	if err != nil {
		log.Println("error caching board page", page, "for", board, err)
	}
}

// regenerate the front page
func (self *RedisCache) regenFrontPageLocal(indexout, boardsout io.Writer) {
	indexbuf := new(bytes.Buffer)
	indexwr := io.MultiWriter(indexout, indexbuf)
	boardsbuf := new(bytes.Buffer)
	boardswr := io.MultiWriter(boardsout, boardsbuf)

	template.genFrontPage(10, self.prefix, self.name, indexwr, boardswr, self.database)

	_, err1 := self.client.Set(INDEX, indexbuf.String(), 0).Result()
	if err1 != nil {
		log.Println("cannot cache front page", err1)
	}

	_, err2 := self.client.Set(BOARDS, boardsbuf.String(), 0).Result()
	if err2 != nil {
		log.Println("cannot render board list page", err2)
		return
	}
}

func (self *RedisCache) RegenFrontPage() {
	self.regenFrontPageLocal(ioutil.Discard, ioutil.Discard)
}

// regenerate the overboard
func (self *RedisCache) regenUkko(out io.Writer) {
	buf := new(bytes.Buffer)
	wr := io.MultiWriter(out, buf)
	template.genUkko(self.prefix, self.name, wr, self.database)
	_, err := self.client.Set(UKKO, buf.String(), 0).Result()
	if err != nil {
		log.Println("error caching ukko", err)
	}
}

// regenerate pages after a mod event
func (self *RedisCache) RegenOnModEvent(newsgroup, msgid, root string, page int) {
	if root == msgid {
		self.DeleteThreadMarkup(root)
	} else {
		self.regenThreadChan <- ArticleEntry{root, newsgroup}
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
	cache.regenBoardTicker = time.NewTicker(time.Second * 10)
	cache.longTermTicker = time.NewTicker(time.Hour)
	cache.ukkoTicker = time.NewTicker(time.Second * 30)
	cache.regenThreadTicker = time.NewTicker(time.Second)
	cache.regenBoardMap = make(map[string]groupRegenRequest)
	cache.regenThreadMap = make(map[string]ArticleEntry)
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
		PoolSize:    1000,
	})

	_, err := cache.client.Ping().Result() //check for successful connection
	if err != nil {
		log.Fatalf("cannot open connection to redis: %s", err)
	}

	return cache
}
