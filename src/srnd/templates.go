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
)

type templateEngine struct {
  // every newsgroup
  groups map[string]GroupModel
  // loaded templates
  templates map[string]string
  // root directory for templates
  template_dir string
}

func (self templateEngine) templateCached(name string) (ok bool) {
  _, ok = self.templates[name]
  return 
}

func (self templateEngine) getTemplate(name string) (t string) {
  if self.templateCached(name) {
    t, _ = self.templates[name]
  } else {
    // ignores errors, this is probably bad
    b, _ := ioutil.ReadFile(filepath.Join(self.template_dir, name))
    t = string(b)
    self.templates[name] = t
  }
  return
}

func (self templateEngine) renderTemplate(name string, obj interface{}) string {
  t := self.getTemplate(name)
  return mustache.Render(t, obj)
}

// get a board model given a newsgroup
// load un updated board model if we don't have it
func (self templateEngine) obtainBoard(prefix, frontend, group string, db Database) (model GroupModel) {
  model, ok := self.groups[group]
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
func (self templateEngine) genBoardPage(prefix, frontend, newsgroup string, page int, outfile string, db Database) {

  // get it
  board := self.obtainBoard(prefix, frontend, newsgroup, db)
  // update it
  board = board.Update(page, db)
  if page >= len(board) {
    log.Println("board page should not exist", newsgroup ,page)
    return
  }
  wr, err := OpenFileWriter(outfile)
  if err == nil {
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
func (self templateEngine) genBoard(prefix, frontend, newsgroup, outdir string, db Database) {
  // get it
  board := self.obtainBoard(prefix, frontend, newsgroup, db)
  // update it
  board = board.UpdateAll(db)
  // save it
  self.groups[newsgroup] = board

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

func (self templateEngine) genUkko(prefix, frontend, outfile string, database Database) {
  // get the last 15 bumped threads globally
  var threads []ThreadModel
  for _, article := range database.GetLastBumpedThreads("", 15) {
    newsgroup, msgid := article[1], article[0]
    // obtain board
    board := self.obtainBoard(prefix, frontend, newsgroup, database)
    board = board.Update(0, database)
    for _, th := range(board[0].Threads()) {
      if th.OP().MessageID() == msgid {
        threads = append(threads, th.Update(database))
        break
      }
    }
    // save state of board
    self.groups[newsgroup] = board
  }
  wr, err := OpenFileWriter(outfile)
  if err == nil {
    io.WriteString(wr, template.renderTemplate("ukko.mustache", map[string]interface{} { "prefix" : prefix, "threads" : threads }))
    wr.Close()
    log.Println("wrote file", outfile)
  } else {
    log.Println("error generating ukko", err)
  }
}

func (self templateEngine) genThread(messageID, prefix, frontend, outfile string, db Database) {

  newsgroup, page, err := db.GetPageForRootMessage(messageID)
  if err != nil {
    log.Println("did not get root post info when regenerating thread", messageID, err)
    return
  }
  // get it
  board := self.obtainBoard(prefix, frontend, newsgroup, db)
  // update our thread
  board[page] = board[page].UpdateThread(messageID, db)
  for _, th := range board[page].Threads() {
    if th.OP().MessageID() == messageID {
      th = th.Update(db)
      // we found it
      wr, err := OpenFileWriter(outfile)
      if err == nil {
        th.RenderTo(wr)
        wr.Close()
        log.Println("wrote file", outfile)
      } else {
        log.Println("did not write", outfile, err)
      }
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



func (self templateEngine) genFrontPage(top_count int, frontend_name, outfile string, db Database) {
  // the graph for the front page
  var frontpage_graph boardPageRows

  // for each group
  groups := db.GetAllNewsgroups()
  for idx, group := range groups {
    if idx >= top_count {
      break
    }
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
  wr, err := OpenFileWriter(outfile)
  if err != nil {
    log.Println("cannot render front page", err)
    return
  }

  param := make(map[string]interface{})
  sort.Sort(frontpage_graph)
  param["graph"] = frontpage_graph
  param["frontend"] = frontend_name
  param["totalposts"] = db.ArticleCount()
  _, err = io.WriteString(wr, self.renderTemplate("frontpage.mustache", param))
  if err != nil {
    log.Println("error writing front page", err)
  }
  wr.Close()
  log.Println("wrote file", outfile)
}
