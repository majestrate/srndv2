//
// model.go
// template model implementation
//
package srnd

import (
  "fmt"
  "io"
  "path/filepath"
  "strings"
  "time"
)


// base model type
type BaseModel interface {

  // site url prefix
  Prefix() string

  // render to a writer
  RenderTo(wr io.Writer) error

}


// for attachments
type AttachmentModel interface {

  BaseModel
  
  Thumbnail() string
  Source() string
  Filename() string
  
}

// for individual posts
type PostModel interface {

  BaseModel

  CSSClass() string
  
  MessageID() string
  PostHash() string
  ShortHash() string
  PostURL() string
  Frontend() string
  Subject() string
  Name() string
  Date() string
  OP() bool
  Attachments() []AttachmentModel
  Board() string
  Sage() bool
  Pubkey() string
  Reference() string
  
  RenderBody() string
  RenderPost() string

  // truncate body to a certain size
  // return copy
  Truncate() PostModel
  
}

// interface for models that have a navbar
type NavbarModel interface {

  Navbar() string

}

// for threads
type ThreadModel interface {

  BaseModel
  NavbarModel
  
  OP() PostModel
  Replies() []PostModel
  Board() string
  BoardURL() string
  // return a short version of the thread
  // does not include all replies
  Truncate() ThreadModel
  // update the thread's replies
  // return the updated model
  Update(db Database) ThreadModel
}

// board interface
// for 1 page on a board
type BoardModel interface {

  BaseModel
  NavbarModel
  
  Frontend() string
  Name() string
  Threads() []ThreadModel

  // JUST update this thread
  // if we don't have it already loaded do nothing
  UpdateThread(message_id string, db Database) BoardModel

  // check if we have this thread
  HasThread(message_id string) bool
  
  // update the board's contents
  // return the updated model
  Update(db Database) BoardModel
}

type LinkModel interface {

  Text() string
  LinkURL() string
}

// newsgroup model
// every page on a newsgroup
type GroupModel []BoardModel

// TODO: optimize using 1 query?
// update every page
// return updated model
func (self GroupModel) UpdateAll(db Database) GroupModel {
  for idx, page := range self {
    self[idx] = page.Update(db)
  }
  return self
}

// update a certain page
// does nothing if out of bounds
func (self GroupModel) Update(page int, db Database) GroupModel {
  if len(self) > page {
    self[page] = self[page].Update(db)
  }
  return self
}


type boardModel struct {
  frontend string
  prefix string
  board string
  page int
  pages int
  threads []ThreadModel
}


func (self boardModel) Navbar() string {
  param := make(map[string]interface{})
  param["name"] = fmt.Sprintf("page %d for %s", self.page, self.board)
  param["frontend"] = self.frontend
  var links []LinkModel
  for i := 0 ; i < self.pages ; i ++ {
    links = append(links, linkModel{
      link: fmt.Sprintf("%s%s-%d.html", self.prefix, self.board, i),
      text: fmt.Sprintf("[ %d ]", i),
    })
  }
  param["prefix"] = self.prefix
  param["links"] = links
  return template.renderTemplate("navbar.mustache", param)
}

func (self boardModel) UpdateThread(messageID string, db Database) BoardModel {

  for idx, th := range self.threads {
    if th.OP().MessageID() == messageID {
      // found it
      self.threads[idx] = th.Update(db)
    }
  }
  return self
}

func (self boardModel) HasThread(messageID string) bool {
  for _, th := range self.threads {
    if th.OP().MessageID() == messageID {
      return true
    }
  }
  return false
}

func (self boardModel) Frontend() string {
  return self.frontend
}

func (self boardModel) Prefix() string {
  return self.prefix
}

func (self boardModel) Name() string {
  return self.board
}

func (self boardModel) Threads() []ThreadModel {
  return self.threads
}

func (self boardModel) RenderTo(wr io.Writer) error {
  param := make(map[string]interface{})
  param["board"] = self
  param["form"] = renderPostForm(self.Prefix(), self.board, "")
  _, err := io.WriteString(wr, template.renderTemplate("board.mustache", param))
  return err
}

// refetch all threads on this page
func (self boardModel) Update(db Database) BoardModel {
  // ignore error
  perpage, _ := db.GetThreadsPerPage(self.board)
  // refetch all on this page
  model := db.GetGroupForPage(self.prefix, self.frontend, self.board, self.page, int(perpage))
  var threads []ThreadModel
  for _, th := range model.Threads() {
    threads = append(threads, th.Update(db))
  }
  return boardModel{
    frontend: self.frontend,
    prefix: self.prefix,
    board: self.board,
    page: self.page,
    pages: self.pages,
    threads: threads,
  }
}

type post struct {
  prefix string
  board string
  name string
  subject string
  message string
  message_rendered string
  message_id string
  path string
  op bool
  posted int64
  parent string
  sage bool
  pubkey string
  reference string
  attachments []AttachmentModel
}

type attachment struct {
  prefix string
  filepath string
  filename string
}

func (self attachment) Prefix() string {
  return self.prefix
}

func (self attachment) RenderTo(wr io.Writer) error {
  // does nothing
  return nil
}

func (self attachment) Thumbnail() string {
  return self.prefix + "thm/" + self.filepath + ".jpg"
}

func (self attachment) Source() string {
  return self.prefix + "img/" + self.filepath
}

func (self attachment) Filename() string {
  return self.filename
}

func PostModelFromMessage(parent, prefix string, nntp NNTPMessage) PostModel {
  p := post{}
  p.name = nntp.Name()
  p.subject = nntp.Subject()
  p.message = nntp.Message()
  p.path = nntp.Path()
  p.message_id = nntp.MessageID()
  p.board = nntp.Newsgroup()
  p.posted = nntp.Posted()
  p.op = nntp.OP()
  p.prefix = prefix
  p.parent = parent
  p.sage = nntp.Sage()
  p.pubkey = nntp.Pubkey()
  for _, att := range nntp.Attachments() {
    p.attachments = append(p.attachments, att.ToModel(prefix))
  }
  return p
}

func (self post) Reference() string {
  return self.parent
}

func (self post) ShortHash() string {
  return ShortHashMessageID(self.message_id)
}

func (self post) Pubkey() string {
  if len(self.pubkey) > 0 {
    return fmt.Sprintf("<label title=\"%s\">%s</label>", self.pubkey, makeTripcode(self.pubkey))
  }
  return ""
}


func (self post) Sage() bool {
  return self.sage
}

func (self post) CSSClass() string {
  if self.OP() {
    return "post op"
  } else {
    return "post reply"
  }
}

func (self post) OP() bool {
  return self.parent == self.message_id || len(self.parent) == 0
}

func (self post) Date() string {
  return time.Unix(self.posted, 0).Format(time.ANSIC)
}

func (self post) TemplateDir() string {
  return filepath.Join("contrib", "templates", "default")
}

func (self post) MessageID() string  {
  return self.message_id
}

func (self post) Frontend() string {
  idx := strings.LastIndex(self.path, "!")
  if idx == -1 {
    return self.path
  }
  return self.path[idx+1:]
}

func (self post) Board() string {
  return self.board
}

func (self post) PostHash() string {
  return HashMessageID(self.message_id)
}

func (self post) Name() string {
  return self.name
}

func (self post) Subject() string {
  return self.subject
}

func (self post) Attachments() []AttachmentModel {
  return self.attachments
}

func (self post) PostURL() string {
  return fmt.Sprintf("%sthread-%s.html#%s", self.Prefix(), ShortHashMessageID(self.parent), self.PostHash())
}

func (self post) Prefix() string {
  return self.prefix 
}

func (self post) RenderTo(wr io.Writer) error {
  _, err := io.WriteString(wr, self.RenderPost())
  return err
}

func (self post) RenderPost() string {
  return template.renderTemplate("post.mustache", self)
}

func (self post) Truncate() PostModel {
  if len(self.message) > 500 {
    message := self.message[:500] + "\n...\n[Post Truncated]\n"
    return post{
      message: message,
      prefix: self.prefix,
      board: self.board,
      name: self.name,
      subject: self.subject,
      message_id: self.message_id,
      path: self.path,
      op: self.op,
      posted: self.posted,
      parent: self.parent,
      sage: self.sage,
      pubkey: self.pubkey,
      reference: self.reference,
      attachments: self.attachments,
    }
  }
  return self
}

func (self post) RenderShortBody() string {
  // TODO: hardcoded limit
  return memeposting(self.message)
}

func (self post) RenderBody() string {
  // :^)
  if len(self.message_rendered) == 0 {
    self.message_rendered = memeposting(self.message)
  }
  return self.message_rendered
}

type thread struct {
  prefix string
  links []LinkModel
  posts []PostModel
}

func (self thread) Prefix() string {
  return self.prefix
}

func (self thread) Navbar() string {
  param := make(map[string]interface{})
  param["name"] = fmt.Sprintf("Thread %s", self.posts[0].ShortHash())
  param["frontend"] = self.Board()
  param["links"] = self.links
  param["prefix"] = self.prefix
  return template.renderTemplate("navbar.mustache", param)
}

func (self thread) Board() string {
  return self.posts[0].Board()
}

func (self thread) BoardURL() string {
  return fmt.Sprintf("%s%s-0.html", self.Prefix(), self.Board())
}

// get our default template dir
func defaultTemplateDir() string {
  return  filepath.Join("contrib", "templates", "default")
}

func (self thread) RenderTo(wr io.Writer) error {
  postform := renderPostForm(self.prefix, self.Board(), self.posts[0].MessageID())
  data := template.renderTemplate("thread.mustache", map[string]interface{} { "thread": self, "form" : postform})
  io.WriteString(wr, data)
  return nil
}

func (self thread) OP() PostModel {
  return self.posts[0]
}

func (self thread) Replies() []PostModel {
  if len(self.posts) > 1 {
    return self.posts[1:]
  }
  return []PostModel{}
}

func (self thread) Truncate() ThreadModel {
  trunc := 5
  if len(self.posts) > trunc {
    return thread{
      links: self.links,
      posts: append([]PostModel{self.posts[0]}, self.posts[len(self.posts)-trunc:]...),
      prefix: self.prefix,
    }
  }
  return self
}

// refetch all replies if anything differs
func (self thread) Update(db Database) ThreadModel {
  root := self.posts[0].MessageID()
  reply_count := db.CountThreadReplies(root)

  if int(reply_count) + 1 != len(self.posts) {

    return thread{
      posts: append([]PostModel{self.posts[0]}, db.GetThreadReplyPostModels(self.prefix, root, 0)...),
      links: self.links,
      prefix: self.prefix,
    }
  }
  return self
}


type linkModel struct {
  text string
  link string
}

func (self linkModel) LinkURL() string {
  return self.link
}

func (self linkModel) Text() string {
  return self.text
}
