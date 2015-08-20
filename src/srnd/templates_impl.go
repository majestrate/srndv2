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
  return renderTemplate("navbar.mustache", param)
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
  pubkey string
  attachments []AttachmentModel
}

type attachment struct {
  prefix string
  thumbnail string
  source string
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
  return self.thumbnail
}

func (self attachment) Source() string {
  return self.source
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

// TODO: implement
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
  return renderTemplate("post.mustache", self)
}

func (self post) Truncate(amount int) {
  if len(self.message) > amount && amount > 0 {
    self.message = self.message[:amount]
  }
}

func (self post) RenderShortBody() string {
  // TODO: hardcoded limit
  return memeposting(self.message)
}

func (self post) RenderBody() string {
  // :^)
  return memeposting(self.message)
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
  param["frontend"] = "nntpchan" // TODO: make this different?
  param["links"] = self.links
  param["prefix"] = self.prefix
  return renderTemplate("navbar.mustache", param)
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
  data := renderTemplate("thread.mustache", map[string]interface{} { "thread": self, "form" : postform})
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

func renderTemplate(name string, obj interface{}) string {
  return mustache.RenderFile(filepath.Join(defaultTemplateDir(), name), obj)
}

func renderUkko(prefix string, threads []ThreadModel) string {
  return renderTemplate("ukko.mustache", map[string]interface{} { "prefix" : prefix, "threads" : threads } )
}


func renderPostForm(prefix, board, op_msg_id string) string {
  url := prefix + "post/" + board
  button := "New Thread"
  if op_msg_id != "" {
    button = "Reply"
  }
  return renderTemplate("postform.mustache", map[string]string { "post_url" : url, "reference" : op_msg_id , "button" : button } )
}
