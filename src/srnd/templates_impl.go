//
// templates_impl.go
// template model implementation
//
package srnd

import (
  "github.com/hoisie/mustache"
  "io"
  "path/filepath"
  "strings"
)

type post struct {
  board string
  name string
  subject string
  message string
  message_id string
  path string
  op bool
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

func PostModelFromMessage(nntp *NNTPMessage) PostModel {
  p :=  post{}
  p.name = nntp.Name
  p.subject = nntp.Subject
  p.message = nntp.Message
  p.path = nntp.Path
  p.message_id = nntp.MessageID
  p.board = nntp.Newsgroup
  p.op = nntp.OP
  return p
}

func (self post) ShortHash() string {
  return ShortHashMessageID(self.message_id)
}

func (self post) OP() bool {
  return self.op
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

// TODO: implement
func (self post) PostURL() string {
  return "#"
}

// TODO: implement
func (self post) Prefix() string {
  return "/" 
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
  ThreadModel
  posts []PostModel
}

// TODO: implement
func (self thread) Prefix() string {
  return "/"
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
  postform := self.renderPostForm(self.Board(), self.posts[0].MessageID())
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

func NewThreadModel(posts []PostModel) ThreadModel {
  th := thread{}
  th.posts = posts
  return th
}

func templateRender(fname string, obj interface{}) string {
  return mustache.RenderFile(fname, obj)
}


func (self thread) renderPostForm(board, op_msg_id string) string {
  // TODO: prefix
  url := "/post/" + board
  return mustache.RenderFile(filepath.Join(defaultTemplateDir(), "postform.mustache"), map[string]string { "post_url" : url, "reference" : op_msg_id , "button" : "Reply" } )
}
