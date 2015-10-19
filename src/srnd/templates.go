//
// templates.go
// template model interfaces
//
package srnd

import (
  "fmt"
  "github.com/hoisie/mustache"
  "io"
  "io/ioutil"
  "log"
  "path/filepath"
  "sort"
  "strings"
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
}

func (self *templateEngine) templateCached(name string) (ok bool) {
  _, ok = self.templates[name]
  return 
}

// explicitly reload a template
func (self *templateEngine) reloadTemplate(name string) {
  self.templates[name] = self.loadTemplate(name)
}

// check if we have this template
func (self *templateEngine) hasTemplate(name string) bool {
  return CheckFile(self.templateFilepath(name))
}

// explicitly reload all loaded templates
func (self *templateEngine) reloadAllTemplates() {
  loadThese := []string{}
  // get all the names of the templates we have loaded
  for tname, _ := range self.templates {
    loadThese = append(loadThese, tname)
  }
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
  for _, group := range template.groups {
    // for each page in group
    for _, page := range group {
      // for each thread in page
      updateLinkCacheForBoard(page)
    }
  }
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
  template.links_short[s] = h
  template.links[h] = u
  // for each reply
  for _, p := range thread.Replies() {
    // put reply entry
    m = p.MessageID()
    u = p.PostURL()
    s = ShorterHashMessageID(m)
    h = ShortHashMessageID(m)
    template.links_short[s] = h
    template.links[h] = u
    template.links[h] = u
  }
}

// get the url for a backlink
func (self *templateEngine) findLink(hash string) (url string) {
  if len(hash) == 8 {
    // short version of hash
    hash, _ = self.links_short[hash]
  }
  url, _ = self.links[hash]
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
  if ! self.templateCached(name) {
    self.templates[name] = self.loadTemplate(name)    
  }
  t, _ = self.templates[name]
  return
}

// render a template, self explanitory
func (self *templateEngine) renderTemplate(name string, obj interface{}) string {
  t := self.getTemplate(name)
  return mustache.Render(t, obj)
}

// get a board model given a newsgroup
// load un updated board model if we don't have it
func (self *templateEngine) obtainBoard(prefix, frontend, group string, db Database) (model GroupModel) {
  model, ok := self.groups[group]
  // if we don't already have the board loaded load it
  if ! ok  {
    p := db.GetGroupPageCount(group)
    pages := int(p)
    // ignore error
    perpage, _ := db.GetThreadsPerPage(group)
    for page := 0 ; page < pages ; page ++ {
      model = append(model, db.GetGroupForPage(prefix, frontend, group, page, int(perpage)))
    }
    self.groups[group] = model
  }
  return

}
// generate a board page
func (self *templateEngine) genBoardPage(prefix, frontend, newsgroup string, page int, outfile string, db Database) {
  // get the board model
  board := self.obtainBoard(prefix, frontend, newsgroup, db)
  // update the board page
  board = board.Update(page, db)
  if page >= len(board) {
    log.Println("board page should not exist", newsgroup, "page", page)
    return
  }
  // render it
  wr, err := OpenFileWriter(outfile)
  if err == nil {
    updateLinkCacheForBoard(board[page])
    board[page].RenderTo(wr)
    wr.Close()
    log.Println("wrote file", outfile)
  } else {
    log.Println("error generating board page", page, "for", newsgroup, err)
  }
  // save it
  self.groups[newsgroup] = board
}

// generate every page for a board
func (self *templateEngine) genBoard(prefix, frontend, newsgroup, outdir string, db Database) {
  // get the board model
  board := self.obtainBoard(prefix, frontend, newsgroup, db)
  // update the entire board model
  board = board.UpdateAll(db)
  // save the model
  self.groups[newsgroup] = board
  updateLinkCache()
  
  pages := len(board)
  for page := 0 ; page < pages ; page ++ {
    outfile := filepath.Join(outdir, fmt.Sprintf("%s-%d.html", newsgroup, page))
    wr, err := OpenFileWriter(outfile)
    if err == nil {
      board[page].RenderTo(wr)
      wr.Close()
      log.Println("wrote file", outfile)
    } else {
      log.Println("error generating board page", page, "for", newsgroup, err)
    }
  }
}

func (self *templateEngine) genUkko(prefix, frontend, outfile string, database Database) {
  var threads []ThreadModel
  // get the last 15 bumped threads globally, for each...
  for _, article := range database.GetLastBumpedThreads("", 15) {
    // get the newsgroup and root post id
    newsgroup, msgid := article[1], article[0]
    // obtain board model
    board := self.obtainBoard(prefix, frontend, newsgroup, database)
    // update first page
    board = board.Update(0, database)
    // grab the root post in question
    th := board[0].GetThread(msgid)
    if th != nil {
        threads = append(threads, th.Update(database))
    }
    // save board model
    self.groups[newsgroup] = board
  }
  wr, err := OpenFileWriter(outfile)
  if err == nil {
    updateLinkCache()
    io.WriteString(wr, template.renderTemplate("ukko.mustache", map[string]interface{} { "prefix" : prefix, "threads" : threads }))
    wr.Close()
  } else {
    log.Println("error generating ukko", err)
  }
}

func (self *templateEngine) genThread(root ArticleEntry, prefix, frontend, outfile string, db Database) {
  newsgroup := root.Newsgroup()
  msgid := root.MessageID()
  var th ThreadModel
  // get the board model
  board := self.obtainBoard(prefix, frontend, newsgroup, db)
  // find the thread model in question
  for _, pagemodel := range board {
    t := pagemodel.GetThread(msgid)
    if t != nil {
      th = t
      break
    }
  }

  if th == nil {
    // a new thread?
    board[0] = board[0].Update(db)
    t := board[0].GetThread(msgid)
    if t != nil {
      th = t
    }
  }
  
  if th == nil {
    log.Println("we didn't find thread for", msgid, "did not regenerate")
  } else {
    // update thread model and write it out
    th = th.Update(db)
    wr, err := OpenFileWriter(outfile)
    if err == nil {
      updateLinkCacheForThread(th)
      th.RenderTo(wr)
      wr.Close()
      log.Println("wrote file", outfile)
    } else {
      log.Println("did not write", outfile, err)
    }
  }
  // save it
  self.groups[newsgroup] = board
}

func newTemplateEngine(dir string) *templateEngine {
  return &templateEngine{
    groups: make(map[string]GroupModel),
    templates: make(map[string]string),
    template_dir: dir,
    links: make(map[string]string),
  }
}

var template = newTemplateEngine(defaultTemplateDir())


func renderPostForm(prefix, board, op_msg_id string) string {
  url := prefix + "post/" + board
  button := "New Thread"
  if op_msg_id != "" {
    button = "Reply"
  }
  return template.renderTemplate("postform.mustache", map[string]string { "post_url" : url, "reference" : op_msg_id , "button" : button } )
}


// generate front page and board list
func (self *templateEngine) genFrontPage(top_count int, frontend_name, outdir string, db Database) {
  // the graph for the front page
  var frontpage_graph boardPageRows

  // for each group
  groups := db.GetAllNewsgroups()
  for _, group := range groups {
    // posts per hour
    hour := db.CountPostsInGroup(group, 3600)
    // posts per day
    day := db.CountPostsInGroup(group, 86400)
    // posts total
    all := db.CountPostsInGroup(group, 0)
    frontpage_graph = append(frontpage_graph, boardPageRow{
      All: all,
      Day: day,
      Hour: hour,
      Board: group,
    })
  }
  wr, err := OpenFileWriter(filepath.Join(outdir, "index.html"))
  if err != nil {
    log.Println("cannot render front page", err)
    return
  }

  param := make(map[string]interface{})
  sort.Sort(frontpage_graph)
  if len(frontpage_graph) < top_count {
    param["graph"] = frontpage_graph
  } else {
    param["graph"] = frontpage_graph[:top_count]
  }
  param["frontend"] = frontend_name
  param["totalposts"] = db.ArticleCount()
  _, err = io.WriteString(wr, self.renderTemplate("frontpage.mustache", param))
  if err != nil {
    log.Println("error writing front page", err)
  }
  wr.Close()
  
  wr, err = OpenFileWriter(filepath.Join(outdir, "boards.html"))
  if err != nil {
    log.Println("cannot render board list page", err)
    return
  }

  param["graph"] = frontpage_graph
  _, err = io.WriteString(wr, self.renderTemplate("boardlist.mustache", param))
  if err != nil {
    log.Println("error writing board list page", err)
  }
  wr.Close()
}
