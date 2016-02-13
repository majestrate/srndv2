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

	IsI2P() bool
	IsTor() bool
	IsClearnet() bool

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

	SetAllowFiles(allow bool)
	AllowFiles() bool
	OP() PostModel
	Replies() []PostModel
	Board() string
	BoardURL() string
	// return a short version of the thread
	// does not include all replies
	Truncate() ThreadModel
	// update the thread's replies
	Update(db Database)
	// is this thread dirty and needing updating?
	IsDirty() bool
	// mark thread as dirty
	MarkDirty()
}

// board interface
// for 1 page on a board
type BoardModel interface {
	BaseModel
	NavbarModel

	Frontend() string
	Name() string
	Threads() []ThreadModel

	AllowFiles() bool
	SetAllowFiles(files bool)

	// JUST update this thread
	// if we don't have it already loaded do nothing
	UpdateThread(message_id string, db Database)

	// get a thread model with this id
	// returns nil if we don't have it
	GetThread(message_id string) ThreadModel

	// put a thread back after updating externally
	PutThread(th ThreadModel)

	// deprecated, use GetThread
	HasThread(message_id string) bool

	// update the board's contents
	Update(db Database)
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
func (self *GroupModel) UpdateAll(db Database) {
	m := *self
	for _, page := range m {
		page.Update(db)
	}
}

// update a certain page
// does nothing if out of bounds
func (self *GroupModel) Update(page int, db Database) {
	m := *self
	if len(m) > page {
		m[page].Update(db)
	}
}

type boardModel struct {
	allowFiles bool
	frontend   string
	prefix     string
	board      string
	page       int
	pages      int
	threads    []ThreadModel
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
	name             string
	subject          string
	message          string
	message_rendered string
	message_id       string
	path             string
	addr             string
	op               bool
	posted           int64
	parent           string
	sage             bool
	pubkey           string
	reference        string
	attachments      []AttachmentModel
}

type attachment struct {
	prefix   string
	filepath string
	filename string
}

func (self *attachment) Prefix() string {
	return self.prefix
}

func (self *attachment) RenderTo(wr io.Writer) error {
	// does nothing
	return nil
}

func (self *attachment) Thumbnail() string {
	return self.prefix + "thm/" + self.filepath + ".jpg"
}

func (self *attachment) Source() string {
	return self.prefix + "img/" + self.filepath
}

func (self *attachment) Filename() string {
	return self.filename
}

func PostModelFromMessage(parent, prefix string, nntp NNTPMessage) PostModel {
	p := new(post)
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
	p.addr = nntp.Addr()
	p.sage = nntp.Sage()
	p.pubkey = nntp.Pubkey()
	for _, att := range nntp.Attachments() {
		p.attachments = append(p.attachments, att.ToModel(prefix))
	}
	return p
}

func (self *post) Reference() string {
	return self.parent
}

func (self *post) ShortHash() string {
	return ShortHashMessageID(self.message_id)
}

func (self *post) Pubkey() string {
	if len(self.pubkey) > 0 {
		return fmt.Sprintf("<label title=\"%s\">%s</label>", self.pubkey, makeTripcode(self.pubkey))
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
	return self.parent == self.message_id || len(self.parent) == 0
}

func (self *post) Date() string {
	return time.Unix(self.posted, 0).Format(time.ANSIC)
}

func (self *post) TemplateDir() string {
	return filepath.Join("contrib", "templates", "default")
}

func (self *post) MessageID() string {
	return self.message_id
}

func (self *post) Frontend() string {
	idx := strings.LastIndex(self.path, "!")
	if idx == -1 {
		return self.path
	}
	return self.path[idx+1:]
}

func (self *post) Board() string {
	return self.board
}

func (self *post) PostHash() string {
	return HashMessageID(self.message_id)
}

func (self *post) Name() string {
	return self.name
}

func (self *post) Subject() string {
	return self.subject
}

func (self *post) Attachments() []AttachmentModel {
	return self.attachments
}

func (self *post) PostURL() string {
	return fmt.Sprintf("%sthread-%s.html#%s", self.Prefix(), HashMessageID(self.parent), self.PostHash())
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
	message := self.message
	subject := self.subject
	name := self.name
	if len(self.message) > 500 {
		message = self.message[:500] + "\n...\n[Post Truncated]\n"
	}
	if len(self.subject) > 100 {
		subject = self.subject[:100] + "..."
	}
	if len(self.name) > 100 {
		name = self.name[:100] + "..."
	}

	return &post{
		prefix:     self.prefix,
		board:      self.board,
		name:       name,
		subject:    subject,
		message:    message,
		message_id: self.message_id,
		path:       self.path,
		addr:       self.addr,
		op:         self.op,
		posted:     self.posted,
		parent:     self.parent,
		sage:       self.sage,
		pubkey:     self.pubkey,
		reference:  self.reference,
		// TODO: copy?
		attachments: self.attachments,
	}
}

func (self *post) RenderShortBody() string {
	// TODO: hardcoded limit
	return memeposting(self.message)
}

func (self *post) RenderBody() string {
	// :^)
	if len(self.message_rendered) == 0 {
		self.message_rendered = memeposting(self.message)
	}
	return self.message_rendered
}

type thread struct {
	allowFiles bool
	prefix     string
	links      []LinkModel
	posts      []PostModel
	dirty      bool
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
	param["name"] = fmt.Sprintf("Thread %s", self.posts[0].ShortHash())
	param["frontend"] = self.Board()
	param["links"] = self.links
	param["prefix"] = self.prefix
	return template.renderTemplate("navbar.mustache", param)
}

func (self *thread) Board() string {
	return self.posts[0].Board()
}

func (self *thread) BoardURL() string {
	return fmt.Sprintf("%s%s-0.html", self.Prefix(), self.Board())
}

// get our default template dir
func defaultTemplateDir() string {
	return filepath.Join("contrib", "templates", "default")
}

func (self *thread) RenderTo(wr io.Writer) error {
	postform := renderPostForm(self.prefix, self.Board(), self.posts[0].MessageID(), self.allowFiles)
	data := template.renderTemplate("thread.mustache", map[string]interface{}{"thread": self, "form": postform})
	io.WriteString(wr, data)
	return nil
}

func (self *thread) OP() PostModel {
	return self.posts[0]
}

func (self *thread) Replies() []PostModel {
	if len(self.posts) > 1 {
		return self.posts[1:]
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
	if len(self.posts) > trunc {
		return &thread{
			allowFiles: self.allowFiles,
			links:      self.links,
			posts:      append([]PostModel{self.posts[0]}, self.posts[len(self.posts)-trunc:]...),
			prefix:     self.prefix,
			dirty:      false,
		}
	}
	return self
}

func (self *thread) Update(db Database) {
	root := self.posts[0].MessageID()
	reply_count := db.CountThreadReplies(root)
	i_reply_count := int(reply_count)
	if len(self.posts) > 1 && i_reply_count > len(self.posts[1:]) {
		// was from a new post(s)
		diff := i_reply_count - len(self.posts[1:])
		newposts := db.GetThreadReplyPostModels(self.prefix, root, i_reply_count-diff, diff)
		self.posts = append(self.posts, newposts...)
	} else {
		// mod event
		self.posts = append([]PostModel{self.posts[0]}, db.GetThreadReplyPostModels(self.prefix, root, 0, 0)...)
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

type boardPageRow struct {
	Board string
	Hour  int64
	Day   int64
	All   int64
}

type boardPageRows []boardPageRow

func (self boardPageRows) Len() int {
	return len(self)
}

func (self boardPageRows) Less(i, j int) bool {
	i_val := self[i]
	j_val := self[j]
	return (i_val.Day + i_val.Hour*24) > (j_val.Day + j_val.Hour*24)
}

func (self boardPageRows) Swap(i, j int) {
	self[i], self[j] = self[j], self[i]
}

type postsGraphRow struct {
	day time.Time
	Num int64
	mag int64
}

func (p *postsGraphRow) GraphRune(r string) (s string) {
	var num int64
	if p.mag > 0 {
		num = p.Num / p.mag
	} else {
		num = p.Num
	}
	for num > 0 {
		s += r
		num--
	}
	return
}

func (p postsGraphRow) Date() (s string) {
	return p.day.String()[:7]
}

func (p postsGraphRow) Day() (s string) {
	return p.day.String()[5:11]
}

func (p postsGraphRow) RegularGraph() (s string) {
	return p.GraphRune("=")
}

// :0========3 overcock :3 graph of data
func (p postsGraphRow) OvercockGraph() (s string) {
	var num int64
	if p.mag > 0 {
		num = p.Num / p.mag
	} else {
		num = p.Num
	}
	if num > 0 {
		s = ":0"
		num -= 1
		for num > 0 {
			s += "="
			num--
		}
		s += "3"
	} else {
		s = ":3"
	}
	return
}

type postsGraph []postsGraphRow

func (graph postsGraph) Render() string {
	return template.renderTemplate("posts_graph.mustache", map[string]interface{}{"graph": graph})
}

func (self postsGraph) Len() int {
	return len(self)
}

func (self postsGraph) Less(i, j int) bool {
	i_val := self[i]
	j_val := self[j]
	return i_val.day.Unix() > j_val.day.Unix()
}

func (self postsGraph) Swap(i, j int) {
	self[i], self[j] = self[j], self[i]
}

func (self postsGraph) Scale() (graph postsGraph) {
	// find max
	max := int64(0)
	for _, p := range self {
		if p.Num > max {
			max = p.Num
		}
	}
	mag := max / 25
	for _, p := range self {
		p.mag = mag
		graph = append(graph, p)
	}
	return
}

type overviewModel []PostModel

func (o overviewModel) Render() string {
	return template.renderTemplate("overview.mustache", map[string]interface{}{"overview": o})
}
