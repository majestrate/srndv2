//
// templates_impl.go
// template model implementation
//
package srnd

import (
  "fmt"
  "github.com/hoisie/mustache"
  "io"
  "path/filepath"
  "strings"
  "time"
)

type boardModel struct {
  frontend string
  prefix string
  board string
  threads []ThreadModel
}


func (self boardModel) RenderNavbar() string {
  // TODO navbar
  return "navbar goes here"
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
  fname := filepath.Join(defaultTemplateDir(), "board.mustache")
  param := make(map[string]interface{})
  param["board"] = self
  param["form"] = renderPostForm(self.Prefix(), self.board, "")
  _, err := io.WriteString(wr, mustache.RenderFile(fname, param))
  return err
}

func createBoardModel(prefix, frontend, name string, threads []ThreadModel) BoardModel {
  return boardModel{frontend, prefix, name, threads}
}

type post struct {
  prefix string
  board string
  name string
  subject string
  message string
  message_id string
  path string
  op bool
  posted int64
  parent string
  sage bool
}

type attachment struct {
  thumbnail string
  source string
  filename string
}

func (self attachment) Thumbnail() string {
  return self.thumbnail
}

func (self attachment) Source() string {
  return self.source
}

func (self attachment) Filename() string {
  return self.filename
}

func PostModelFromMessage(parent, prefix string, nntp NNTPMessage) PostModel {
  p :=  post{}
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
  return p
}

func (self post) ShortHash() string {
  return ShortHashMessageID(self.message_id)
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

// TODO: implement
func (self post) Attachments() []AttachmentModel {
  return nil
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
  fname := filepath.Join(self.TemplateDir(), "post.mustache")
  return mustache.RenderFile(fname, self)
}

// TODO: formatting
func (self post) RenderBody() string {
  // escape it
  return mustache.Render("{{message}}", map[string]string { "message": self.message})
}

type thread struct {
  prefix string
  posts []PostModel
}

func (self thread) Prefix() string {
  return self.prefix
}

func (self thread) Board() string {
  return self.posts[0].Board()
}

// get our default template dir
func defaultTemplateDir() string {
  return  filepath.Join("contrib", "templates", "default")
}

func (self thread) TemplateDir() string {
  return defaultTemplateDir()
}

func (self thread) RenderTo(wr io.Writer) error {
  fname := filepath.Join(self.TemplateDir(), "thread.mustache")
  rpls := self.Replies()
  postform := renderPostForm(self.prefix, self.Board(), self.posts[0].MessageID())
  data := mustache.RenderFile(fname, map[string]interface{} { "thread": self, "repls" : rpls, "form" : postform})
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

func NewThreadModel(prefix string, posts []PostModel) ThreadModel {
  th := thread{}
  th.posts = posts
  th.prefix = prefix
  return th
}

func templateRender(fname string, obj interface{}) string {
  return mustache.RenderFile(fname, obj)
}

func renderTemplate(name string, obj interface{}) string {
  return templateRender(filepath.Join(defaultTemplateDir(), name), obj)
}


func renderUkko(prefix string, threads []ThreadModel) string {
  return renderTemplate("ukko.mustache", map[string]interface{} { "prefix" : prefix, "threads" : threads } )
}


func renderPostForm(prefix, board, op_msg_id string) string {
  url := prefix + "post/" + board
  return renderTemplate("postform.mustache", map[string]string { "post_url" : url, "reference" : op_msg_id , "button" : "Reply" } )
}
