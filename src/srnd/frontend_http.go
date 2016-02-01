//
// frontend_http.go
//
// srnd http frontend implementation
//
package srnd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dchest/captcha"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/gorilla/websocket"
	"github.com/majestrate/nacl"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type bannedFunc func()
type errorFunc func(error)
type successFunc func(NNTPMessage)

// an attachment in a post
type postAttachment struct {
	Filename string `json:"name"`
	Filedata string `json:"data"`
	Filetype string `json:"type"`
}

// an api post request
type postRequest struct {
	Reference    string            `json:"reference"`
	Name         string            `json:"name"`
	Email        string            `json:"email"`
	Subject      string            `json:"subject"`
	Frontend     string            `json:"frontend"`
	Attachment   postAttachment    `json:"file"`
	Group        string            `json:"newsgroup"`
	IpAddress    string            `json:"ip"`
	Destination  string            `json:"i2p"`
	Dubs         bool              `json:"dubs"`
	Message      string            `json:"message"`
	ExtraHeaders map[string]string `json:"headers"`
}

// regenerate a newsgroup page
type groupRegenRequest struct {
	// which newsgroup
	group string
	// page number
	page int
}

type httpFrontend struct {
	modui        ModUI
	httpmux      *mux.Router
	daemon       *NNTPDaemon
	postchan     chan NNTPMessage
	recvpostchan chan NNTPMessage
	bindaddr     string
	name         string

	webroot_dir  string
	template_dir string
	static_dir   string

	regen_threads  int
	regen_on_start bool
	attachments    bool

	prefix          string
	regenThreadChan chan ArticleEntry
	regenGroupChan  chan groupRegenRequest
	regenBoard      map[string]groupRegenRequest

	regenBoardTicker *time.Ticker
	ukkoTicker       *time.Ticker
	longTermTicker   *time.Ticker

	store *sessions.CookieStore

	upgrader websocket.Upgrader

	jsonUsername string
	jsonPassword string
	enableJson   bool
}

// do we allow this newsgroup?
func (self httpFrontend) AllowNewsgroup(group string) bool {
	// XXX: hardcoded nntp prefix
	// TODO: make configurable nntp prefix
	return strings.HasPrefix(group, "overchan.") && newsgroupValidFormat(group) || group == "ctl" && group != "overchan."
}

// try to delete root post's page
func (self httpFrontend) deleteThreadMarkup(root_post_id string) {
	fname := self.getFilenameForThread(root_post_id)
	log.Println("delete file", fname)
	os.Remove(fname)
}

func (self httpFrontend) getFilenameForThread(root_post_id string) string {
	fname := fmt.Sprintf("thread-%s.html", ShortHashMessageID(root_post_id))
	return filepath.Join(self.webroot_dir, fname)
}

func (self httpFrontend) deleteBoardMarkup(group string) {
	pages, _ := self.daemon.database.GetPagesPerBoard(group)
	for page := 0; page < pages; page++ {
		fname := self.getFilenameForBoardPage(group, page)
		log.Println("delete file", fname)
		os.Remove(fname)
	}
}

func (self httpFrontend) getFilenameForBoardPage(boardname string, pageno int) string {
	fname := fmt.Sprintf("%s-%d.html", boardname, pageno)
	return filepath.Join(self.webroot_dir, fname)
}

func (self httpFrontend) NewPostsChan() chan NNTPMessage {
	return self.postchan
}

func (self httpFrontend) PostsChan() chan NNTPMessage {
	return self.recvpostchan
}

// regen every newsgroup
func (self httpFrontend) regenAll() {
	log.Println("regen all on http frontend")

	// get all groups
	groups := self.daemon.database.GetAllNewsgroups()
	if groups != nil {
		for _, group := range groups {
			// send every thread for this group down the regen thread channel
			go self.daemon.database.GetGroupThreads(group, self.regenThreadChan)
			pages := self.daemon.database.GetGroupPageCount(group)
			var pg int64
			for pg = 0; pg < pages; pg++ {
				self.regenGroupChan <- groupRegenRequest{group, int(pg)}
			}
		}
	}
}

func (self httpFrontend) regenLongTerm() {
	template.genGraphs(self.prefix, self.webroot_dir, self.daemon.database)
}

func (self httpFrontend) pollLongTerm() {
	for {
		<-self.longTermTicker.C
		// regenerate long term stuff
		self.regenLongTerm()
	}
}

func (self httpFrontend) pollRegen() {
	for {
		select {
		// listen for regen board requests
		case req := <-self.regenGroupChan:
			self.regenBoard[fmt.Sprintf("%s|%s", req.group, req.page)] = req
			// listen for regen thread requests
		case entry := <-self.regenThreadChan:
			self.regenerateThread(entry)
			// regen ukko
		case _ = <-self.ukkoTicker.C:
			self.regenUkko()
			self.regenFrontPage()
		case _ = <-self.regenBoardTicker.C:
			for _, v := range self.regenBoard {
				self.regenerateBoardPage(v.group, v.page)
			}
			self.regenBoard = make(map[string]groupRegenRequest)
		}
	}
}

func (self httpFrontend) poll() {

	// regenerate front page
	self.regenFrontPage()

	// trigger regen
	if self.regen_on_start {
		self.regenAll()
	}

	modChnl := self.modui.MessageChan()
	for {
		select {
		case nntp := <-modChnl:
			// forward signed messages to daemon
			self.postchan <- nntp
		case nntp := <-self.recvpostchan:
			// get root post and tell frontend to regen that thread
			msgid := nntp.MessageID()
			group := nntp.Newsgroup()
			if len(nntp.Reference()) > 0 {
				msgid = nntp.Reference()
			}
			self.regenThreadChan <- ArticleEntry{msgid, group}
			// regen the newsgroup we're in
			// TODO: regen only what we need to
			pages := self.daemon.database.GetGroupPageCount(group)
			// regen all pages
			var page int64
			for ; page < pages; page++ {
				req := groupRegenRequest{
					group: group,
					page:  int(page),
				}
				self.regenGroupChan <- req
			}
		}
	}
}

// create a new captcha, return as json object
func (self httpFrontend) new_captcha_json(wr http.ResponseWriter, r *http.Request) {
	captcha_id := captcha.New()
	resp := make(map[string]string)
	// the captcha id
	resp["id"] = captcha_id
	// url of the image
	resp["url"] = fmt.Sprintf("%s%s.png", self.prefix, captcha_id)
	enc := json.NewEncoder(wr)
	enc.Encode(&resp)
}

// regen every page of the board
func (self httpFrontend) regenerateBoard(group string) {
	template.genBoard(self.prefix, self.name, group, self.webroot_dir, self.daemon.database)
}

// regenerate just a thread page
func (self httpFrontend) regenerateThread(root ArticleEntry) {
	msgid := root.MessageID()
	if self.daemon.store.HasArticle(msgid) {
		log.Println("rengerate thread", msgid)
		fname := self.getFilenameForThread(msgid)
		template.genThread(root, self.prefix, self.name, fname, self.daemon.database)
	} else {
		log.Println("don't have root post", msgid, "not regenerating thread")
	}
}

// regenerate just a page on a board
func (self httpFrontend) regenerateBoardPage(board string, page int) {
	fname := self.getFilenameForBoardPage(board, page)
	template.genBoardPage(self.prefix, self.name, board, page, fname, self.daemon.database)
}

// regenerate the front page
func (self httpFrontend) regenFrontPage() {
	template.genFrontPage(10, self.prefix, self.name, self.webroot_dir, self.daemon.database)
}

// regenerate the overboard
func (self httpFrontend) regenUkko() {
	fname := filepath.Join(self.webroot_dir, "ukko.html")
	template.genUkko(self.prefix, self.name, fname, self.daemon.database)
}

// regenerate pages after a mod event
func (self httpFrontend) regenOnModEvent(newsgroup, msgid, root string, page int) {
	if root == msgid {
		fname := self.getFilenameForThread(root)
		log.Println("remove file", fname)
		os.Remove(fname)
	} else {
		self.regenThreadChan <- ArticleEntry{root, newsgroup}
	}
	self.regenGroupChan <- groupRegenRequest{newsgroup, int(page)}
}

// handle newboard page
func (self httpFrontend) handle_newboard(wr http.ResponseWriter, r *http.Request) {
	param := make(map[string]string)
	param["prefix"] = self.prefix
	io.WriteString(wr, template.renderTemplate("newboard.mustache", param))
}

// handle new post via http request for a board
func (self httpFrontend) handle_postform(wr http.ResponseWriter, r *http.Request, board string) {

	// the post we will turn into an nntp article
	var pr postRequest

	mp_reader, err := r.MultipartReader()

	if err != nil {
		wr.WriteHeader(500)
		io.WriteString(wr, err.Error())
		return
	}

	pr.Group = board

	// encrypt IP Addresses
	// when a post is recv'd from a frontend, the remote address is given its own symetric key that the local srnd uses to encrypt the address with, for privacy
	// when a mod event is fired, it includes the encrypted IP address and the symetric key that frontend used to encrypt it, thus allowing others to determine the IP address
	// each stnf will optionally comply with the mod event, banning the address from being able to post from that frontend
	// this will be done eventually but for now that requires too much infrastrucutre, let's go with regular IP Addresses for now.

	// get the "real" ip address from the request

	pr.IpAddress, _, err = net.SplitHostPort(r.RemoteAddr)
	// TODO: have in config upstream proxy ip and check for that
	if strings.HasPrefix(pr.IpAddress, "127.") {
		// if it's loopback check headers for reverse proxy headers
		// TODO: make sure this isn't a tor user being sneaky
		pr.IpAddress = getRealIP(r.Header.Get("X-Real-IP"))
	}
	pr.Destination = r.Header.Get("X-I2P-DestHash")
	pr.Frontend = self.name

	var captcha_retry bool
	var captcha_solution, captcha_id string
	var att_filename, att_mime string
	var att_buff bytes.Buffer
	var att NNTPAttachment
	var url string
	url = fmt.Sprintf("%s-0.html", board)
	var part_buff bytes.Buffer
	for {
		part, err := mp_reader.NextPart()
		if err == nil {
			// get the name of the part
			partname := part.FormName()

			// read part for attachment
			if partname == "attachment" && self.attachments {
				log.Println("attaching file...")
				att = readAttachmentFromMimePart(part)
				continue
			}
			io.Copy(&part_buff, part)

			// check for values we want
			if partname == "subject" {
				pr.Subject = part_buff.String()
			} else if partname == "name" {
				pr.Name = part_buff.String()
			} else if partname == "message" {
				pr.Message = part_buff.String()
			} else if partname == "reference" {
				pr.Reference = part_buff.String()
				if len(pr.Reference) == 0 {
					url = fmt.Sprintf("%s-0.html", board)
				} else {
					url = fmt.Sprintf("thread-%s.html", ShortHashMessageID(pr.Reference))
				}
			} else if partname == "captcha_id" {
				captcha_id = part_buff.String()
			} else if partname == "captcha" {
				captcha_solution = part_buff.String()
			} else if partname == "attachment_data" {
				// repost of data
				dec := base64.NewDecoder(base64.StdEncoding, &part_buff)
				_, err = io.Copy(&att_buff, dec)
			} else if partname == "attachment_filename" {
				att_filename = part_buff.String()
			} else if partname == "attachment_mime" {
				att_mime = part_buff.String()
			} else if partname == "dubs" {
				pr.Dubs = part_buff.String() == "on"
			}

			// we done
			// reset buffer for reading parts
			part_buff.Reset()
			// close our part
			part.Close()
		} else {
			if err != io.EOF {
				errmsg := fmt.Sprintf("httpfrontend post handler error reading multipart: %s", err)
				log.Println(errmsg)
				wr.WriteHeader(500)
				io.WriteString(wr, errmsg)
				return
			}
			break
		}
	}

	if len(captcha_id) == 0 {
		s, _ := self.store.Get(r, self.name)
		cid, ok := s.Values["captcha_id"]
		if ok {
			captcha_id = cid.(string)
		}
		s.Values["captcha_id"] = ""
		s.Save(r, wr)
	}

	if !captcha.VerifyString(captcha_id, captcha_solution) {
		// captcha is not valid
		captcha_retry = true
	}

	// make error template param
	resp_map := make(map[string]string)
	resp_map["prefix"] = self.prefix
	// set redirect url
	if len(url) > 0 {
		// if we explicitly know the url use that
		resp_map["redirect_url"] = self.prefix + url
	} else {
		// if our referer is saying we are from /new/ page use that
		// otherwise use prefix
		if strings.HasSuffix(r.Referer(), self.prefix+"new/") {
			resp_map["redirect_url"] = self.prefix + "new/"
		} else {
			resp_map["redirect_url"] = self.prefix
		}
	}

	if att_buff.Len() > 0 && len(att_filename) > 0 && len(att_mime) > 0 {
		att = createAttachment(att_mime, att_filename, &att_buff)
	}

	if captcha_retry {
		// retry the post with a new captcha
		wr.WriteHeader(200)
		resp_map = make(map[string]string)
		resp_map["subject"] = pr.Subject
		resp_map["name"] = pr.Name
		resp_map["message"] = pr.Message
		resp_map["reference"] = pr.Reference
		if att == nil {
			// no attachments
		} else {
			// 1 attachment
			var buff bytes.Buffer
			enc := base64.NewEncoder(base64.StdEncoding, &buff)
			_, err = io.Copy(enc, att)
			enc.Close()
			resp_map["attachment"] = buff.String()
			resp_map["attachment_filename"] = att.Filename()
			resp_map["attachment_type"] = att.Mime()
		}
		c := captcha.New()
		resp_map["captcha_id"] = c
		s, _ := self.store.Get(r, self.name)
		s.Values["captcha_id"] = c
		s.Save(r, wr)
		resp_map["fail_message"] = "try posting again"
		resp_map["prefix"] = self.prefix
		io.WriteString(wr, template.renderTemplate("post_retry.mustache", resp_map))
		return
	}

	if att != nil {
		pr.Attachment = postAttachment{
			Filename: att.Filename(),
			Filetype: att.Mime(),
			Filedata: att.Filedata(),
		}
	}

	b := func() {
		wr.WriteHeader(403)
		io.WriteString(wr, "nigguh u banned")
	}

	e := func(err error) {
		wr.WriteHeader(200)
		resp_map["reason"] = err.Error()
		resp_map["prefix"] = self.prefix
		resp_map["redirect_url"] = self.prefix + url
		io.WriteString(wr, template.renderTemplate("post_fail.mustache", resp_map))
	}

	s := func(nntp NNTPMessage) {
		// send success reply
		wr.WriteHeader(200)
		// determine the root post so we can redirect to the thread for it
		msg_id := nntp.Headers().Get("References", nntp.MessageID())
		// render response as success
		url := fmt.Sprintf("%sthread-%s.html", self.prefix, ShortHashMessageID(msg_id))
		io.WriteString(wr, template.renderTemplate("post_success.mustache", map[string]string{"prefix": self.prefix, "message_id": nntp.MessageID(), "redirect_url": url}))
	}
	self.handle_postRequest(&pr, b, e, s)
}

// turn a post request into an nntp article write it to temp dir and tell daemon
func (self httpFrontend) handle_postRequest(pr *postRequest, b bannedFunc, e errorFunc, s successFunc) {
	var err error
	var nntp nntpArticle
	var banned bool
	nntp.headers = make(ArticleHeaders)
	address := pr.IpAddress
	// check for banned
	if len(address) > 0 {
		banned, err = self.daemon.database.CheckIPBanned(address)
		if err == nil {
			if banned {
				b()
				return
			}
		} else {
			e(err)
			return
		}
	}
	if len(address) == 0 {
		address = "Tor"
	}

	if !strings.HasPrefix(address, "127.") {
		// set the ip address of the poster to be put into article headers
		// if we cannot determine it, i.e. we are on Tor/i2p, this value is not set
		if address == "Tor" {
			nntp.headers.Set("X-Tor-Poster", "1")
		} else {
			address, err = self.daemon.database.GetEncAddress(address)
			if err == nil {
				nntp.headers.Set("X-Encrypted-IP", address)
			} else {
				e(err)
				return
			}
			// TODO: add x-tor-poster header for tor exits
		}
	}

	// always lower case newsgroups
	board := strings.ToLower(pr.Group)

	// post fail message
	banned, err = self.daemon.database.NewsgroupBanned(board)
	if banned {
		e(errors.New("newsgroup banned "))
		return
	}
	if err != nil {
		e(err)
	}

	if !self.daemon.database.HasNewsgroup(board) {
		e(errors.New("we don't have this newsgroup " + board))
		return
	}

	// if we don't have an address for the poster try checking for i2p httpd headers
	if len(pr.Destination) == i2pDestHashLen() {
		nntp.headers.Set("X-I2P-DestHash", pr.Destination)
	}

	ref := pr.Reference
	if len(ref) > 0 {
		if ValidMessageID(ref) {
			if self.daemon.database.HasArticleLocal(ref) {
				nntp.headers.Set("References", ref)
			} else {
				e(errors.New("article referenced not locally available"))
				return
			}
		} else {
			e(errors.New("invalid reference"))
			return
		}
	}

	// set newsgroup
	nntp.headers.Set("Newsgroups", pr.Group)

	// check message size
	if len(pr.Attachment.Filedata) == 0 && len(pr.Message) == 0 {
		e(errors.New("no message"))
		return
	} else if len(pr.Message) > 1024*1024*10 {
		e(errors.New("your message is too big"))
		return
	}

	if len(pr.Frontend) == 0 {
		// :-DDD
		pr.Frontend = "mongo.db.is.web.scale"
	}

	subject := pr.Subject

	// set subject
	if len(subject) == 0 {
		subject = "None"
	}
	nntp.headers.Set("Subject", subject)
	if isSage(subject) {
		nntp.headers.Set("X-Sage", "1")
	}

	name := pr.Name

	var tripcode_privkey []byte

	// set name
	if len(name) == 0 {
		name = "Anonymous"
	} else {
		idx := strings.Index(name, "#")
		// tripcode
		if idx >= 0 {
			tripcode_privkey = parseTripcodeSecret(name[idx+1:])
			name = strings.Trim(name[:idx], "\t ")
			if name == "" {
				name = "Anonymous"
			}
		}
	}
	msgid := genMessageID(pr.Frontend)
	// roll until dubs if desired
	for pr.Dubs && !MessageIDWillDoDubs(msgid) {
		msgid = genMessageID(pr.Frontend)
	}

	nntp.headers.Set("From", nntpSanitize(fmt.Sprintf("%s <poster@%s>", name, pr.Frontend)))
	nntp.headers.Set("Message-ID", msgid)

	// set message
	nntp.message = createPlaintextAttachment(pr.Message)
	// set date
	nntp.headers.Set("Date", timeNowStr())
	// append path from frontend
	nntp.AppendPath(pr.Frontend)

	// add extra headers if needed
	if pr.ExtraHeaders != nil {
		for name, val := range pr.ExtraHeaders {
			// don't overwrite existing headers
			if nntp.headers.Get(name, "") == "" {
				nntp.headers.Set(name, val)
			}
		}
	}

	att := pr.Attachment

	// add attachment
	if self.attachments && len(att.Filedata) > 0 {
		nntp = nntp.Attach(createAttachment(att.Filetype, att.Filename, strings.NewReader(att.Filedata))).(nntpArticle)
	}
	// pack it before sending so that the article is well formed
	nntp.Pack()

	// sign if needed
	if len(tripcode_privkey) == nacl.CryptoSignSeedLen() {
		nntp, err = signArticle(nntp, tripcode_privkey)
		if err != nil {
			// error signing
			e(err)
			return
		}
	}
	// success
	s(nntp)
	// store in temp
	f := self.daemon.store.CreateTempFile(nntp.MessageID())
	if f != nil {
		nntp.WriteTo(f, "\n")
		f.Close()
		// tell daemon
		self.daemon.infeed_load <- nntp.MessageID()
	}
}

// handle posting / postform
func (self httpFrontend) handle_poster(wr http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	var board string
	// extract board
	parts := strings.Count(path, "/")
	if parts > 1 {
		board = strings.Split(path, "/")[2]
	}

	// this is a POST request
	if r.Method == "POST" && self.AllowNewsgroup(board) && newsgroupValidFormat(board) {
		self.handle_postform(wr, r, board)
	} else {
		wr.WriteHeader(403)
		io.WriteString(wr, "Nope")
	}
}

func (self httpFrontend) new_captcha(wr http.ResponseWriter, r *http.Request) {
	s, err := self.store.Get(r, self.name)
	if err == nil {
		captcha_id := captcha.New()
		s.Values["captcha_id"] = captcha_id
		s.Save(r, wr)
		redirect_url := fmt.Sprintf("%scaptcha/%s.png", self.prefix, captcha_id)

		// redirect to the image
		http.Redirect(wr, r, redirect_url, 302)
	} else {
		// todo: send a "this is broken" image
		wr.WriteHeader(500)
	}
}

// send error
func api_error(wr http.ResponseWriter, err error) {
	resp := make(map[string]string)
	resp["error"] = err.Error()
	wr.Header().Add("Content-Type", "text/json; encoding=UTF-8")
	enc := json.NewEncoder(wr)
	enc.Encode(resp)
}

// authenticated part of api
// handle all functions that require authentication
func (self httpFrontend) handle_authed_api(wr http.ResponseWriter, r *http.Request, api string) {
	// check valid format
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	mtype, _, err := mime.ParseMediaType(ct)
	if err == nil {
		if strings.HasSuffix(mtype, "/json") {
			// valid :^)
		} else {
			// bad content type
			api_error(wr, errors.New(fmt.Sprintf("invalid content type: %s", ct)))
			return
		}
	} else {
		// bad content type
		api_error(wr, err)
		return
	}

	b := func() {
		api_error(wr, errors.New("banned"))
	}

	e := func(err error) {
		api_error(wr, err)
	}

	s := func(nntp NNTPMessage) {
		resp := make(map[string]string)
		resp["id"] = nntp.MessageID()
		enc := json.NewEncoder(wr)
		enc.Encode(resp)
	}

	dec := json.NewDecoder(r.Body)
	if api == "post" {
		var pr postRequest
		err = dec.Decode(&pr)
		if err == nil {
			// we parsed it
			self.handle_postRequest(&pr, b, e, s)
		} else {
			// bad parsing?
			api_error(wr, err)
		}
	} else {
		// no such method
		wr.WriteHeader(404)
	}
}

// handle un authenticated part of api
func (self httpFrontend) handle_unauthed_api(wr http.ResponseWriter, r *http.Request, api string) {
	var err error
	if api == "header" {
		var msgids []string
		q := r.URL.Query()
		name := q.Get("name")
		val := q.Get("value")
		msgids, err = self.daemon.database.GetMessageIDByHeader(name, val)
		if err == nil {
			enc := json.NewEncoder(wr)
			enc.Encode(msgids)
		} else {
			api_error(wr, err)
		}
	}
}

func (self httpFrontend) handle_api(wr http.ResponseWriter, r *http.Request) {
	if self.enableJson {
		vars := mux.Vars(r)
		meth := vars["meth"]
		if r.Method == "POST" {
			u, p, ok := r.BasicAuth()
			if ok && u == self.jsonUsername && p == self.jsonPassword {
				// authenticated
				self.handle_authed_api(wr, r, meth)
			} else {
				// invalid auth
				wr.WriteHeader(401)
			}
		} else if r.Method == "GET" {
			self.handle_unauthed_api(wr, r, meth)
		} else {
			// wtf?
			wr.WriteHeader(405)
		}
	} else {
		// not enabled, gone
		wr.WriteHeader(410)
	}
}

func (self httpFrontend) Mainloop() {
	EnsureDir(self.webroot_dir)
	if !CheckFile(self.template_dir) {
		log.Fatalf("no such template folder %s", self.template_dir)
	}

	template.changeTemplateDir(self.template_dir)

	threads := self.regen_threads

	// check for invalid number of threads
	if threads <= 0 {
		threads = 1
	}

	// set up handler mux
	self.httpmux = mux.NewRouter()

	// create mod ui
	self.modui = createHttpModUI(self)

	// modui handlers
	self.httpmux.Path("/mod/").HandlerFunc(self.modui.ServeModPage).Methods("GET")
	self.httpmux.Path("/mod/keygen").HandlerFunc(self.modui.HandleKeyGen).Methods("GET")
	self.httpmux.Path("/mod/login").HandlerFunc(self.modui.HandleLogin).Methods("POST")
	self.httpmux.Path("/mod/del/{article_hash}").HandlerFunc(self.modui.HandleDeletePost).Methods("GET")
	self.httpmux.Path("/mod/ban/{address}").HandlerFunc(self.modui.HandleBanAddress).Methods("GET")
	self.httpmux.Path("/mod/unban/{address}").HandlerFunc(self.modui.HandleUnbanAddress).Methods("GET")
	self.httpmux.Path("/mod/addkey/{pubkey}").HandlerFunc(self.modui.HandleAddPubkey).Methods("GET")
	self.httpmux.Path("/mod/delkey/{pubkey}").HandlerFunc(self.modui.HandleDelPubkey).Methods("GET")
	self.httpmux.Path("/mod/admin/{action}").HandlerFunc(self.modui.HandleAdminCommand).Methods("GET", "POST")
	// webroot handler
	self.httpmux.Path("/").Handler(http.FileServer(http.Dir(self.webroot_dir)))
	self.httpmux.Path("/thm/{f}").Handler(http.FileServer(http.Dir(self.webroot_dir)))
	self.httpmux.Path("/img/{f}").Handler(http.FileServer(http.Dir(self.webroot_dir)))
	self.httpmux.Path("/{f}.html").Handler(http.FileServer(http.Dir(self.webroot_dir)))
	self.httpmux.Path("/static/{f}").Handler(http.FileServer(http.Dir(self.static_dir)))
	// post handler
	self.httpmux.Path("/post/{f}").HandlerFunc(self.handle_poster).Methods("POST")
	// captcha handlers
	self.httpmux.Path("/captcha/img").HandlerFunc(self.new_captcha).Methods("GET")
	self.httpmux.Path("/captcha/{f}").Handler(captcha.Server(350, 175)).Methods("GET")
	self.httpmux.Path("/captcha/new.json").HandlerFunc(self.new_captcha_json).Methods("GET")
	// helper handlers
	self.httpmux.Path("/new/").HandlerFunc(self.handle_newboard).Methods("GET")
	// api handler
	self.httpmux.Path("/api/{meth}").HandlerFunc(self.handle_api).Methods("POST", "GET")
	var err error

	// poll channels
	go self.poll()

	// use N threads for regeneration
	// XXX: will this make it crash when accessing the templates?
	// yes it does
	// for threads > 0 {
	//  go self.pollRegen()
	//  threads --
	// }
	go self.pollRegen()

	// run daemon's mod engine with our frontend
	go RunModEngine(self.daemon.mod, self.regenOnModEvent)

	// run long term regen jobs
	go self.regenLongTerm()

	// start webserver here
	log.Printf("frontend %s binding to %s", self.name, self.bindaddr)

	// serve it!
	err = http.ListenAndServe(self.bindaddr, self.httpmux)
	if err != nil {
		log.Fatalf("failed to bind frontend %s %s", self.name, err)
	}
}

func (self httpFrontend) Regen(msg ArticleEntry) {
	self.regenThreadChan <- msg
	self.regenerateBoard(msg.Newsgroup())
}

// create a new http based frontend
func NewHTTPFrontend(daemon *NNTPDaemon, config map[string]string, url string) Frontend {
	var front httpFrontend
	front.daemon = daemon
	front.regenBoardTicker = time.NewTicker(time.Second * 10)
	front.longTermTicker = time.NewTicker(time.Hour)
	front.ukkoTicker = time.NewTicker(time.Second * 30)
	front.regenBoard = make(map[string]groupRegenRequest)
	front.attachments = mapGetInt(config, "allow_files", 1) == 1
	front.bindaddr = config["bind"]
	front.name = config["name"]
	front.webroot_dir = config["webroot"]
	front.static_dir = config["static_files"]
	front.template_dir = config["templates"]
	front.prefix = config["prefix"]
	front.regen_on_start = config["regen_on_start"] == "1"
	front.regen_threads = mapGetInt(config, "regen_threads", 1)
	if config["json-api"] == "1" {
		front.jsonUsername = config["json-api-username"]
		front.jsonPassword = config["json-api-password"]
		front.enableJson = true
	}
	front.store = sessions.NewCookieStore([]byte(config["api-secret"]))
	front.store.Options = &sessions.Options{
		// TODO: detect http:// etc in prefix
		Path:   front.prefix,
		MaxAge: 600,
	}
	front.postchan = make(chan NNTPMessage, 16)
	front.recvpostchan = make(chan NNTPMessage, 16)
	front.regenThreadChan = make(chan ArticleEntry, 16)
	front.regenGroupChan = make(chan groupRegenRequest, 8)
	return front
}
