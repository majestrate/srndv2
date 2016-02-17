//
// model_mem.go
//
// models held in memory
//
package srnd

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

type boardModel struct {
	allowFiles bool
	frontend   string
	prefix     string
	board      string
	page       int
	pages      int
	threads    []ThreadModel
}

func (self *boardModel) MarshalJSON() (b []byte, err error) {
	return json.Marshal(self.threads)
}

func (self *boardModel) SetAllowFiles(allow bool) {
	self.allowFiles = allow
}

func (self *boardModel) AllowFiles() bool {
	return self.allowFiles
}

func (self *boardModel) PutThread(th ThreadModel) {
	idx := -1
	for i, t := range self.threads {
		if th.OP().MessageID() == t.OP().MessageID() {
			idx = i
			break
		}
	}
	if idx != -1 {
		self.threads[idx] = th
	}
}

func (self *boardModel) Navbar() string {
	param := make(map[string]interface{})
	param["name"] = fmt.Sprintf("page %d for %s", self.page, self.board)
	param["frontend"] = self.frontend
	var links []LinkModel
	for i := 0; i < self.pages; i++ {
		links = append(links, linkModel{
			link: fmt.Sprintf("%s%s-%d.html", self.prefix, self.board, i),
			text: fmt.Sprintf("[ %d ]", i),
		})
	}
	param["prefix"] = self.prefix
	param["links"] = links
	return template.renderTemplate("navbar.mustache", param)
}

func (self *boardModel) Board() string {
	return self.board
}

func (self *boardModel) PageList() []LinkModel {
	var links []LinkModel
	for i := 0; i < self.pages; i++ {
		links = append(links, linkModel{
			link: fmt.Sprintf("%s%s-%d.html", self.prefix, self.board, i),
			text: fmt.Sprintf("%d", i),
		})
	}
	return links
}

func (self *boardModel) UpdateThread(messageID string, db Database) {

	for _, th := range self.threads {
		if th.OP().MessageID() == messageID {
			// found it
			th.Update(db)
			break
		}
	}
}

func (self *boardModel) GetThread(messageID string) ThreadModel {
	for _, th := range self.threads {
		if th.OP().MessageID() == messageID {
			return th
		}
	}
	return nil
}

func (self *boardModel) HasThread(messageID string) bool {
	return self.GetThread(messageID) != nil
}

func (self *boardModel) Frontend() string {
	return self.frontend
}

func (self *boardModel) Prefix() string {
	return self.prefix
}

func (self *boardModel) Name() string {
	return self.board
}

func (self *boardModel) Threads() []ThreadModel {
	return self.threads
}

func (self *boardModel) RenderTo(wr io.Writer) error {
	param := make(map[string]interface{})
	param["board"] = self
	param["form"] = renderPostForm(self.Prefix(), self.board, "", self.allowFiles)
	_, err := io.WriteString(wr, template.renderTemplate("board.mustache", param))
	return err
}

// refetch all threads on this page
func (self *boardModel) Update(db Database) {
	// ignore error
	perpage, _ := db.GetThreadsPerPage(self.board)
	// refetch all on this page
	model := db.GetGroupForPage(self.prefix, self.frontend, self.board, self.page, int(perpage))
	for _, th := range model.Threads() {
		// XXX: do we really need to update it again?
		th.Update(db)
	}
	self.threads = model.Threads()
}

type post struct {
	prefix           string
	board            string
	PostName         string
	PostSubject      string
	PostMessage      string
	message_rendered string
	Message_id       string
	MessagePath      string
	addr             string
	op               bool
	Posted           int64
	Parent           string
	sage             bool
	Key              string
	Files            []AttachmentModel
}

func (self *post) MarshalJSON() (b []byte, err error) {
	return json.Marshal(*self)
}

type attachment struct {
	prefix string
	Path   string
	Name   string
}

func (self *attachment) MarshalJSON() (b []byte, err error) {
	return json.Marshal(*self)
}

func (self *attachment) Prefix() string {
	return self.prefix
}

func (self *attachment) RenderTo(wr io.Writer) error {
	// does nothing
	return nil
}

func (self *attachment) Thumbnail() string {
	if strings.HasSuffix(self.Path, ".gif") {
		return self.prefix + "thm/" + self.Path
	}
	return self.prefix + "thm/" + self.Path + ".jpg"
}

func (self *attachment) Source() string {
	return self.prefix + "img/" + self.Path
}

func (self *attachment) Filename() string {
	return self.Name
}

func PostModelFromMessage(parent, prefix string, nntp NNTPMessage) PostModel {
	p := new(post)
	p.PostName = nntp.Name()
	p.PostSubject = nntp.Subject()
	p.PostMessage = nntp.Message()
	p.MessagePath = nntp.Path()
	p.Message_id = nntp.MessageID()
	p.board = nntp.Newsgroup()
	p.Posted = nntp.Posted()
	p.op = nntp.OP()
	p.prefix = prefix
	p.Parent = parent
	p.addr = nntp.Addr()
	p.sage = nntp.Sage()
	p.Key = nntp.Pubkey()
	for _, att := range nntp.Attachments() {
		p.Files = append(p.Files, att.ToModel(prefix))
	}
	return p
}

func (self *post) Reference() string {
	return self.Parent
}

func (self *post) ShortHash() string {
	return ShortHashMessageID(self.MessageID())
}

func (self *post) Pubkey() string {
	if len(self.Key) > 0 {
		return fmt.Sprintf("<label title=\"%s\">%s</label>", self.Key, makeTripcode(self.Key))
	}
	return ""
}

func (self *post) Sage() bool {
	return self.sage
}

func (self *post) CSSClass() string {
	if self.OP() {
		return "post op"
	} else {
		return "post reply"
	}
}

func (self *post) OP() bool {
	return self.Parent == self.Message_id || len(self.Parent) == 0
}

func (self *post) Date() string {
	return time.Unix(self.Posted, 0).Format(time.ANSIC)
}

func (self *post) DateRFC() string {
	return time.Unix(self.Posted, 0).Format(time.RFC3339)
}

func (self *post) TemplateDir() string {
	return filepath.Join("contrib", "templates", "default")
}

func (self *post) MessageID() string {
	return self.Message_id
}

func (self *post) Frontend() string {
	idx := strings.LastIndex(self.MessagePath, "!")
	if idx == -1 {
		return self.MessagePath
	}
	return self.MessagePath[idx+1:]
}

func (self *post) Board() string {
	return self.board
}

func (self *post) PostHash() string {
	return HashMessageID(self.Message_id)
}

func (self *post) Name() string {
	return self.PostName
}

func (self *post) Subject() string {
	return self.PostSubject
}

func (self *post) Attachments() []AttachmentModel {
	return self.Files
}

func (self *post) PostURL() string {
	return fmt.Sprintf("%sthread-%s.html#%s", self.Prefix(), HashMessageID(self.Parent), self.PostHash())
}

func (self *post) Prefix() string {
	return self.prefix
}

func (self *post) IsClearnet() bool {
	return len(self.addr) == encAddrLen()
}

func (self *post) IsI2P() bool {
	return len(self.addr) == i2pDestHashLen()
}

func (self *post) IsTor() bool {
	return len(self.addr) == 0
}

func (self *post) RenderTo(wr io.Writer) error {
	_, err := io.WriteString(wr, self.RenderPost())
	return err
}

func (self *post) RenderPost() string {
	return template.renderTemplate("post.mustache", self)
}

func (self *post) Truncate() PostModel {
	message := self.PostMessage
	subject := self.PostSubject
	name := self.PostName
	if len(message) > 500 {
		message = message[:500] + "\n...\n[Post Truncated]\n"
	}
	if len(subject) > 100 {
		subject = subject[:100] + "..."
	}
	if len(name) > 100 {
		name = name[:100] + "..."
	}

	return &post{
		prefix:      self.prefix,
		board:       self.board,
		PostName:    name,
		PostSubject: subject,
		PostMessage: message,
		Message_id:  self.Message_id,
		MessagePath: self.MessagePath,
		addr:        self.addr,
		op:          self.op,
		Posted:      self.Posted,
		Parent:      self.Parent,
		sage:        self.sage,
		Key:         self.Key,
		// TODO: copy?
		Files: self.Files,
	}
}

func (self *post) RenderShortBody() string {
	return memeposting(self.PostMessage)
}

func (self *post) RenderBody() string {
	// :^)
	if len(self.message_rendered) == 0 {
		self.message_rendered = memeposting(self.PostMessage)
	}
	return self.message_rendered
}

type thread struct {
	allowFiles bool
	prefix     string
	links      []LinkModel
	Posts      []PostModel
	dirty      bool
}

func (self *thread) MarshalJSON() (b []byte, err error) {
	return json.Marshal(self.Posts)
}

func (self *thread) IsDirty() bool {
	return self.dirty
}

func (self *thread) MarkDirty() {
	self.dirty = true
}

func (self *thread) Prefix() string {
	return self.prefix
}

func (self *thread) Navbar() string {
	param := make(map[string]interface{})
	param["name"] = fmt.Sprintf("Thread %s", self.Posts[0].ShortHash())
	param["frontend"] = self.Board()
	param["links"] = self.links
	param["prefix"] = self.prefix
	return template.renderTemplate("navbar.mustache", param)
}

func (self *thread) Board() string {
	return self.Posts[0].Board()
}

func (self *thread) BoardURL() string {
	return fmt.Sprintf("%s%s-0.html", self.Prefix(), self.Board())
}

// get our default template dir
func defaultTemplateDir() string {
	return filepath.Join("contrib", "templates", "default")
}

func (self *thread) RenderTo(wr io.Writer) error {
	postform := renderPostForm(self.prefix, self.Board(), self.Posts[0].MessageID(), self.allowFiles)
	data := template.renderTemplate("thread.mustache", map[string]interface{}{"thread": self, "form": postform})
	io.WriteString(wr, data)
	return nil
}

func (self *thread) OP() PostModel {
	return self.Posts[0]
}

func (self *thread) Replies() []PostModel {
	if len(self.Posts) > 1 {
		return self.Posts[1:]
	}
	return []PostModel{}
}

func (self *thread) AllowFiles() bool {
	return self.allowFiles
}

func (self *thread) SetAllowFiles(allow bool) {
	self.allowFiles = allow
}

func (self *thread) Truncate() ThreadModel {
	trunc := 5
	if len(self.Posts) > trunc {
		return &thread{
			allowFiles: self.allowFiles,
			links:      self.links,
			Posts:      append([]PostModel{self.Posts[0]}, self.Posts[len(self.Posts)-trunc:]...),
			prefix:     self.prefix,
			dirty:      false,
		}
	}
	return self
}

func (self *thread) Update(db Database) {
	root := self.Posts[0].MessageID()
	reply_count := db.CountThreadReplies(root)
	i_reply_count := int(reply_count)
	if len(self.Posts) > 1 && i_reply_count > len(self.Posts[1:]) {
		// was from a new post(s)
		diff := i_reply_count - len(self.Posts[1:])
		newposts := db.GetThreadReplyPostModels(self.prefix, root, i_reply_count-diff, diff)
		self.Posts = append(self.Posts, newposts...)
	} else {
		// mod event
		self.Posts = append([]PostModel{self.Posts[0]}, db.GetThreadReplyPostModels(self.prefix, root, 0, 0)...)
	}
	self.dirty = false
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
