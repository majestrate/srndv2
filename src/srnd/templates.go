//
// templates.go
// template model interfaces
//
package srnd

import (
	"encoding/json"
	"github.com/cbroglie/mustache"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type templateEngine struct {
	// posthash -> url
	links map[string]string
	// shorthash -> posthash
	links_short map[string]string
	// every newsgroup
	groups map[string]GroupModel
	// loaded templates
	templates map[string]string
	// root directory for templates
	template_dir string
	// mutex for accessing links
	links_mtx sync.RWMutex
	// mutex for accessing shortlinks
	links_short_mtx sync.RWMutex
	// mutex for accessing groups
	groups_mtx sync.RWMutex
	// mutex for accessing templates
	templates_mtx sync.RWMutex
}

func (self *templateEngine) templateCached(name string) (ok bool) {
	self.templates_mtx.Lock()
	_, ok = self.templates[name]
	self.templates_mtx.Unlock()
	return
}

// explicitly reload a template
func (self *templateEngine) reloadTemplate(name string) {
	self.templates_mtx.Lock()
	self.templates[name] = self.loadTemplate(name)
	self.templates_mtx.Unlock()
}

// check if we have this template
func (self *templateEngine) hasTemplate(name string) bool {
	return CheckFile(self.templateFilepath(name))
}

// explicitly reload all loaded templates
func (self *templateEngine) reloadAllTemplates() {
	loadThese := []string{}
	// get all the names of the templates we have loaded
	self.templates_mtx.Lock()
	for tname, _ := range self.templates {
		loadThese = append(loadThese, tname)
	}
	self.templates_mtx.Unlock()
	// for each template we have loaded, reload the contents from file
	for _, tname := range loadThese {
		self.reloadTemplate(tname)
	}
}

// update the link -> url cache given our current model
func updateLinkCache() {
	// clear existing cache
	template.links = make(map[string]string)
	// for each group
	template.groups_mtx.Lock()
	for _, group := range template.groups {
		// for each page in group
		for _, page := range group {
			// for each thread in page
			updateLinkCacheForBoard(page)
		}
	}
	template.groups_mtx.Unlock()
}

// update the link -> url cache given a board page
func updateLinkCacheForBoard(page BoardModel) {
	// for each thread in page
	for _, thread := range page.Threads() {
		updateLinkCacheForThread(thread)
	}
}

// update link -> url cache given a thread
func updateLinkCacheForThread(thread ThreadModel) {
	m := thread.OP().MessageID()
	u := thread.OP().PostURL()
	s := ShorterHashMessageID(m)
	h := ShortHashMessageID(m)
	template.links_mtx.Lock()
	template.links_short_mtx.Lock()
	template.links_short[s] = u
	template.links[h] = u
	// for each reply
	for _, p := range thread.Replies() {
		// put reply entry
		m = p.MessageID()
		u = p.PostURL()
		s = ShorterHashMessageID(m)
		h = ShortHashMessageID(m)
		template.links_short[s] = u
		template.links[h] = u
	}
	template.links_mtx.Unlock()
	template.links_short_mtx.Unlock()
}

// get cached post model from cache after updating it
func (self *templateEngine) updatePostModel(prefix, frontend, msgid, rootmsgid, group string, db Database) PostModel {
	// get board
	self.groups_mtx.Lock()
	board := self.groups[group]
	self.groups_mtx.Unlock()

	var th ThreadModel
	if msgid == rootmsgid {
		// new thread
		if len(board) > 0 {
			page := board[0]
			page.Update(db)
			th = page.GetThread(rootmsgid)
		}
	} else {
		// reply
		for _, page := range board {
			t := page.GetThread(rootmsgid)
			if t != nil {
				th = t
				th.Update(db)
				break
			}
		}
	}
	if th == nil {
		// reload board, this will be a heavy operation
		board.UpdateAll(db)
		// find it
		for _, page := range board {
			t := page.GetThread(rootmsgid)
			if t != nil {
				th = t
				th.Update(db)
				break
			}
		}
		for _, page := range board {
			updateLinkCacheForBoard(page)
		}
		self.groups_mtx.Lock()
		self.groups[group] = board
		self.groups_mtx.Unlock()
	}
	if th == nil {
		log.Println("template could not find post model for thread", rootmsgid, "in", group)
		return nil
	}

	// found
	m := th.OP()
	if m.MessageID() == msgid {
		return m
	}
	for _, p := range th.Replies() {
		if p.MessageID() == msgid {
			// found as reply
			return p
		}
	}
	log.Println("template could not find post model for thread", rootmsgid, "in", group)
	// not found
	return nil
}

// get the url for a backlink
func (self *templateEngine) findLink(hash string) (url string) {
	if len(hash) == 10 {

		template.links_short_mtx.Lock()
		// short version of hash
		url, _ = self.links_short[hash]
		template.links_short_mtx.Unlock()
	} else {
		template.links_mtx.Lock()
		url, _ = self.links[hash]
		template.links_mtx.Unlock()
	}
	return
}

// get the filepath to a template
func (self *templateEngine) templateFilepath(name string) string {
	if strings.Count(name, "..") > 0 {
		return ""
	}
	return filepath.Join(self.template_dir, name)
}

// load a template from file, return as string
func (self *templateEngine) loadTemplate(name string) (t string) {
	b, err := ioutil.ReadFile(self.templateFilepath(name))
	if err == nil {
		t = string(b)
	} else {
		log.Println("error loading template", err)
		t = err.Error()
	}
	return
}

// get a template, if it's not cached load from file and cache it
func (self *templateEngine) getTemplate(name string) (t string) {
	if !self.templateCached(name) {
		self.templates_mtx.Lock()
		self.templates[name] = self.loadTemplate(name)
		self.templates_mtx.Unlock()
	}
	self.templates_mtx.Lock()
	t, _ = self.templates[name]
	self.templates_mtx.Unlock()
	return
}

// render a template, self explanitory
func (self *templateEngine) renderTemplate(name string, obj map[string]interface{}) string {
	t := self.getTemplate(name)
	obj["i18n"] = i18nProvider
	s, err := mustache.Render(t, obj)
	if err == nil {
		return s
	} else {
		return err.Error()
	}
}

// write a template to an io.Writer
func (self *templateEngine) writeTemplate(name string, obj map[string]interface{}, wr io.Writer) (err error) {
	_, err = io.WriteString(wr, self.renderTemplate(name, obj))
	return
}

// easy wrapper for json.NewEncoder
func (self *templateEngine) renderJSON(wr io.Writer, obj interface{}) {
	err := json.NewEncoder(wr).Encode(obj)
	if err != nil {
		log.Println("error rendering json", err)
	}
}

// get a board model given a newsgroup
// load un updated board model if we don't have it
func (self *templateEngine) obtainBoard(prefix, frontend, group string, update bool, db Database) (model GroupModel) {
	// warning, we attempt to do smart reloading
	// dark magic may lurk here
	self.groups_mtx.Lock()
	var ok bool
	model, ok = self.groups[group]
	self.groups_mtx.Unlock()
	if ok && !update {
		// we gud
		return
	}
	p := db.GetGroupPageCount(group)
	pages := int(p)
	// model is not up to date
	if update || (!ok) || len(model) < pages {
		perpage, _ := db.GetThreadsPerPage(group)
		// reload all the pages
		var newModel GroupModel
		for page := 0; page < pages; page++ {
			newModel = append(newModel, db.GetGroupForPage(prefix, frontend, group, page, int(perpage)))
		}
		model = newModel
	}

	self.groups_mtx.Lock()
	self.groups[group] = model
	self.groups_mtx.Unlock()

	return
}

func (self *templateEngine) genCatalog(prefix, frontend, group string, wr io.Writer, db Database) {
	board := self.obtainBoard(prefix, frontend, group, false, db)
	catalog := new(catalogModel)
	catalog.prefix = prefix
	catalog.frontend = frontend
	catalog.board = group

	for page, bm := range board {
		for _, th := range bm.Threads() {
			th.Update(db)
			catalog.threads = append(catalog.threads, &catalogItemModel{op: th.OP(), page: page, replycount: len(th.Replies())})
		}
	}
	self.writeTemplate("catalog.mustache", map[string]interface{}{"board": catalog}, wr)
}

// generate a board page
func (self *templateEngine) genBoardPage(allowFiles bool, prefix, frontend, newsgroup string, page int, wr io.Writer, db Database, json bool) {
	// get the board model
	board := self.obtainBoard(prefix, frontend, newsgroup, false, db)
	// update the board page
	if len(board) > 0 {
		board.Update(page, db)
		if page >= len(board) {
			log.Println("board page should not exist", newsgroup, "page", page)
			return
		}
	} else {
		// get the entire board
		board = self.obtainBoard(prefix, frontend, newsgroup, true, db)
		if page >= len(board) {
			log.Println("board page should not exist", newsgroup, "page", page, "tried 2 times to generate page")
			return
		}
	}
	// update link cache
	updateLinkCacheForBoard(board[page])
	// render it
	if json {
		p := board[page]
		self.renderJSON(wr, p)
	} else {
		form := renderPostForm(prefix, newsgroup, "", allowFiles)
		self.writeTemplate("board.mustache", map[string]interface{}{"board": board[page], "page": page, "form": form}, wr)
	}
}

// prepare generation of every page for a board
func (self *templateEngine) prepareGenBoard(allowFiles bool, prefix, frontend, newsgroup string, db Database) int {
	// get the board model
	board := self.obtainBoard(prefix, frontend, newsgroup, true, db)
	// save the model
	self.groups_mtx.Lock()
	self.groups[newsgroup] = board
	self.groups_mtx.Unlock()
	updateLinkCache()

	return len(board)
}

func (self *templateEngine) genUkko(prefix, frontend string, wr io.Writer, database Database, json bool) {
	var threads []ThreadModel
	// get the last 15 bumped threads globally, for each...
	for _, article := range database.GetLastBumpedThreads("", 15) {
		// get the newsgroup and root post id
		newsgroup := article[1]
		// get first thread
		page := database.GetGroupForPage(prefix, frontend, newsgroup, 0, 10)
		for _, t := range page.Threads() {
			if t.OP().MessageID() == article[0] {
				t.Update(database)
				threads = append(threads, t)
				break
			}
		}
	}
	updateLinkCache()
	obj := map[string]interface{}{"prefix": prefix, "threads": threads}
	if json {
		self.renderJSON(wr, obj)
	} else {
		// render ukko navbar
		navbar := make(map[string]interface{})
		navbar["name"] = "Overboard"
		navbar["frontend"] = frontend
		navbar["prefix"] = prefix
		// inject navbar
		obj["navbar"] = self.renderTemplate("navbar.mustache", navbar)
		// render
		self.writeTemplate("ukko.mustache", obj, wr)
	}
}

func (self *templateEngine) genThread(allowFiles bool, root ArticleEntry, prefix, frontend string, wr io.Writer, db Database, json bool) {
	newsgroup := root.Newsgroup()
	msgid := root.MessageID()
	// get the board model, don't update the board
	board := self.obtainBoard(prefix, frontend, newsgroup, false, db)
	// find the thread model in question
	for _, pagemodel := range board {
		t := pagemodel.GetThread(msgid)
		if t != nil {
			// update thread
			t.Update(db)
			// render it
			if json {
				self.renderJSON(wr, t)
			} else {
				form := renderPostForm(prefix, newsgroup, msgid, allowFiles)
				self.writeTemplate("thread.mustache", map[string]interface{}{"thread": t, "board": pagemodel, "form": form}, wr)
			}
			return
		}
	}
	// we didn't find it D:
	// reload everything
	// TODO: should we reload everything!?
	b := self.obtainBoard(prefix, frontend, newsgroup, true, db)
	// find the thread model in question
	for _, pagemodel := range b {
		t := pagemodel.GetThread(msgid)
		if t != nil {
			// we found it
			// render thread
			t.Update(db)
			if json {
				self.renderJSON(wr, t)
			} else {
				form := renderPostForm(prefix, newsgroup, msgid, allowFiles)
				self.writeTemplate("thread.mustache", map[string]interface{}{"thread": t, "board": pagemodel, "form": form}, wr)
			}
			self.groups_mtx.Lock()
			self.groups[newsgroup] = b
			self.groups_mtx.Unlock()
			return
		}
	}
	// it's not there wtf
	log.Println("thread not found for message id", msgid)
}

// change the directory we are using for templates
func (self *templateEngine) changeTemplateDir(dirname string) {
	log.Println("change template directory to", dirname)
	self.template_dir = dirname
	self.reloadAllTemplates()
}

func (self *templateEngine) createNotFoundHandler(prefix, frontend string) (h http.Handler) {
	h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		self.renderNotFound(w, r, prefix, frontend)
	})
	return
}

// default renderer of 404 pages
func (self *templateEngine) renderNotFound(wr http.ResponseWriter, r *http.Request, prefix, frontend string) {
	wr.WriteHeader(404)
	opts := make(map[string]interface{})
	opts["prefix"] = prefix
	opts["frontend"] = frontend
	self.writeTemplate("404.mustache", opts, wr)
}

// preload all boards / threads / replies
func (self *templateEngine) loadAllModels(prefix, frontend string, db Database) {
	groups := db.GetAllNewsgroups()
	for _, group := range groups {
		log.Println("preload models for", group)
		board := self.obtainBoard(prefix, frontend, group, true, db)
		board.UpdateAll(db)
	}
}



func newTemplateEngine(dir string) *templateEngine {
	return &templateEngine{
		groups:       make(map[string]GroupModel),
		templates:    make(map[string]string),
		template_dir: dir,
		links:        make(map[string]string),
		links_short:  make(map[string]string),
	}
}

var template = newTemplateEngine(defaultTemplateDir())

func renderPostForm(prefix, board, op_msg_id string, files bool) string {
	url := prefix + "post/" + board
	button := "New Thread"
	if op_msg_id != "" {
		button = "Reply"
	}
	return template.renderTemplate("postform.mustache", map[string]interface{}{"post_url": url, "reference": op_msg_id, "button": button, "files": files, "prefix": prefix})
}

// generate misc graphs
func (self *templateEngine) genGraphs(prefix string, wr io.Writer, db Database) {

	//
	// begin gen history.html
	//

	var all_posts postsGraph
	// this may take a bit
	posts := db.GetMonthlyPostHistory()

	if posts == nil {
		// wtf?
	} else {
		for _, entry := range posts {
			all_posts = append(all_posts, postsGraphRow{
				day: entry.Time(),
				Num: entry.Count(),
			})
		}
	}
	sort.Sort(all_posts)

	_, err := io.WriteString(wr, self.renderTemplate("graph_history.mustache", map[string]interface{}{"history": all_posts}))
	if err != nil {
		log.Println("error writing history graph", err)
	}

	//
	// end gen history.html
	//

}

// generate front page and board list
func (self *templateEngine) genFrontPage(top_count int, prefix, frontend_name string, indexwr, boardswr io.Writer, db Database) {
	// the graph for the front page
	var frontpage_graph boardPageRows

	// for each group
	groups := db.GetAllNewsgroups()
	for _, group := range groups {
		// posts this hour
		hour := db.CountPostsInGroup(group, 3600)
		// posts today
		day := db.CountPostsInGroup(group, 86400)
		// posts total
		all := db.CountPostsInGroup(group, 0)
		frontpage_graph = append(frontpage_graph, boardPageRow{
			All:   all,
			Day:   day,
			Hour:  hour,
			Board: group,
		})
	}

	var posts_graph postsGraph

	posts := db.GetLastDaysPosts(10)
	if posts == nil {
		// wtf?
	} else {
		for _, entry := range posts {
			posts_graph = append(posts_graph, postsGraphRow{
				day: entry.Time(),
				Num: entry.Count(),
			})
		}
	}

	models := db.GetLastPostedPostModels(prefix, 20)

	wr := indexwr

	param := make(map[string]interface{})

	param["overview"] = self.renderTemplate("overview.mustache", map[string]interface{}{"overview": overviewModel(models)})

	sort.Sort(posts_graph)
	param["postsgraph"] = self.renderTemplate("posts_graph.mustache", map[string]interface{}{"graph": posts_graph})
	sort.Sort(frontpage_graph)
	if len(frontpage_graph) > top_count {
		param["boardgraph"] = frontpage_graph[:top_count]
	} else {
		param["boardgraph"] = frontpage_graph
	}
	param["frontend"] = frontend_name
	param["totalposts"] = db.ArticleCount()

	// render and inject navbar
	param["navbar"] = self.renderTemplate("navbar.mustache", map[string]interface{}{"name": "Front Page", "frontend": frontend_name, "prefix": prefix})

	_, err := io.WriteString(wr, self.renderTemplate("frontpage.mustache", param))
	if err != nil {
		log.Println("error writing front page", err)
	}

	wr = boardswr
	param["graph"] = frontpage_graph
	_, err = io.WriteString(wr, self.renderTemplate("boardlist.mustache", param))
	if err != nil {
		log.Println("error writing board list page", err)
	}
}
