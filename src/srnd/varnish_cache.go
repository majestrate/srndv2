package srnd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

type varnishHandler struct {
	cache *VarnishCache
}

func (self *varnishHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	_, file := filepath.Split(path)

	isjson := strings.HasSuffix(file, "json")

	if strings.HasPrefix(path, "/t/") {
		// thread handler
		hash := strings.Trim(path[3:], "/")
		msg, err := self.cache.database.GetMessageIDByHash(hash)
		if err == nil {
			template.genThread(self.cache.attachments, msg, self.cache.prefix, self.cache.name, w, self.cache.database, isjson)
			return
		} else {
			goto notfound
		}
	}
	if strings.Trim(path, "/") == "overboard" {
		// generate ukko aka overboard
		template.genUkko(self.cache.prefix, self.cache.name, w, self.cache.database, isjson)
		return
	}

	if strings.HasPrefix(path, "/b/") {
		// board handler
		parts := strings.Split(path[3:], "/")
		page := 0
		group := parts[0]
		log.Println(parts)
		if len(parts) == 2 && parts[1] != "" {
			var err error
			page, err = strconv.Atoi(parts[1])
			if err != nil {
				goto notfound
			}
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

	if len(file) == 0 || file == "index.html" {
		template.genFrontPage(10, self.cache.prefix, self.cache.name, w, ioutil.Discard, self.cache.database)
		return
	}

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

type VarnishCache struct {
	database Database
	store    ArticleStore

	webroot_dir string
	name        string

	regen_threads int
	attachments   bool

	prefix          string
	varnish_url     string
	client          *http.Client
	regenThreadChan chan ArticleEntry
	regenGroupChan  chan groupRegenRequest
}

func (self *VarnishCache) invalidate(r string) {
	u, _ := url.Parse(r)
	resp, err := self.client.Do(&http.Request{
		Method: "PURGE",
		URL:    u,
	})
	if err == nil {
		resp.Body.Close()
	} else {
		log.Println("varnish cache error", err)
	}
}

func (self *VarnishCache) DeleteBoardMarkup(group string) {
	n, _ := self.database.GetPagesPerBoard(group)
	for n > 0 {
		go self.invalidate(fmt.Sprintf("%s%s%s-%d.html", self.varnish_url, self.prefix, group, n))
		go self.invalidate(fmt.Sprintf("%s%sb/%s/%d/", self.varnish_url, self.prefix, group, n))
		n--
	}
	self.invalidate(fmt.Sprintf("%s%sb/%s/", self.varnish_url, self.prefix, group))
}

// try to delete root post's page
func (self *VarnishCache) DeleteThreadMarkup(root_post_id string) {
	self.invalidate(fmt.Sprintf("%s%sthread-%s.html", self.varnish_url, self.prefix, HashMessageID(root_post_id)))
	self.invalidate(fmt.Sprintf("%s%st/%s/", self.varnish_url, self.prefix, HashMessageID(root_post_id)))
}

// regen every newsgroup
func (self *VarnishCache) RegenAll() {
	// we will do this as it's used by rengen on start for frontend
	groups := self.database.GetAllNewsgroups()
	for _, group := range groups {
		self.database.GetGroupThreads(group, self.regenThreadChan)
	}
}

func (self *VarnishCache) RegenFrontPage() {
	self.invalidate(fmt.Sprintf("%s%s", self.varnish_url, self.prefix))
}

func (self *VarnishCache) invalidateUkko() {
	// TODO: invalidate paginated ukko
	self.invalidate(fmt.Sprintf("%s%sukko.html", self.varnish_url, self.prefix))
	self.invalidate(fmt.Sprintf("%s%soverboard/", self.varnish_url, self.prefix))
}

func (self *VarnishCache) pollRegen() {
	for {
		select {
		// consume regen requests
		case ev := <-self.regenGroupChan:
			{
				self.invalidate(fmt.Sprintf("%s%s%s-%d.html", self.varnish_url, self.prefix, ev.group, ev.page))
				self.invalidate(fmt.Sprintf("%s%sb/%s/%d/", self.varnish_url, self.prefix, ev.group, ev.page))
				if ev.page == 0 {
					self.invalidate(fmt.Sprintf("%s%sb/%s/", self.varnish_url, self.prefix, ev.group))
				}
			}
		case ev := <-self.regenThreadChan:
			{
				self.Regen(ev)
			}
		}
	}
}

// regen every page of the board
func (self *VarnishCache) RegenerateBoard(group string) {
	n, _ := self.database.GetPagesPerBoard(group)
	for n > 0 {
		go self.invalidate(fmt.Sprintf("%s%s%s-%d.html", self.varnish_url, self.prefix, group, n))
		go self.invalidate(fmt.Sprintf("%s%s%s/%d/", self.varnish_url, self.prefix, group, n))
		n--
	}
	self.invalidate(fmt.Sprintf("%s%sb/%s/", self.varnish_url, self.prefix, group))
}

// regenerate pages after a mod event
func (self *VarnishCache) RegenOnModEvent(newsgroup, msgid, root string, page int) {
	self.regenGroupChan <- groupRegenRequest{newsgroup, page}
	self.regenThreadChan <- ArticleEntry{newsgroup, root}
}

func (self *VarnishCache) Start() {
	go self.pollRegen()
}

func (self *VarnishCache) Regen(msg ArticleEntry) {
	go self.invalidate(fmt.Sprintf("%s%s%s-%d.html", self.varnish_url, self.prefix, msg.Newsgroup(), 0))
	go self.invalidate(fmt.Sprintf("%s%s%s/%d/", self.varnish_url, self.prefix, msg.Newsgroup(), 0))
	go self.invalidate(fmt.Sprintf("%s%sthread-%s.html", self.varnish_url, self.prefix, HashMessageID(msg.MessageID())))
	go self.invalidate(fmt.Sprintf("%s%st/%s/", self.varnish_url, self.prefix, HashMessageID(msg.MessageID())))
	self.invalidateUkko()
}

func (self *VarnishCache) GetThreadChan() chan ArticleEntry {
	return self.regenThreadChan
}

func (self *VarnishCache) GetGroupChan() chan groupRegenRequest {
	return self.regenGroupChan
}

func (self *VarnishCache) GetHandler() http.Handler {
	return &varnishHandler{self}
}

func (self *VarnishCache) Close() {
	//nothig to do
}

func NewVarnishCache(varnish_url, bind_addr, prefix, webroot, name string, attachments bool, db Database, store ArticleStore) CacheInterface {
	cache := new(VarnishCache)
	cache.regenThreadChan = make(chan ArticleEntry, 16)
	cache.regenGroupChan = make(chan groupRegenRequest, 8)
	local_addr, err := net.ResolveTCPAddr("tcp", bind_addr)
	if err != nil {
		log.Fatalf("failed to resolve %s for varnish cache: %s", bind_addr, err)
	}
	cache.client = &http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (c net.Conn, err error) {
				var remote_addr *net.TCPAddr
				remote_addr, err = net.ResolveTCPAddr(network, addr)
				if err == nil {
					c, err = net.DialTCP(network, local_addr, remote_addr)
				}
				return
			},
		},
	}
	cache.prefix = prefix
	cache.webroot_dir = webroot
	cache.name = name
	cache.attachments = attachments
	cache.database = db
	cache.store = store
	cache.varnish_url = varnish_url
	return cache
}
