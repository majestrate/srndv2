//
// frontend_http.go
//
// srnd http frontend implementation
//
package srnd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dchest/captcha"
	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/gorilla/websocket"
	"github.com/majestrate/nacl"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/textproto"
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
	Attachments  []postAttachment  `json:"files"`
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

type liveChan struct {
	postchnl   chan PostModel
	uuid       string
	resultchnl chan *liveChan
}

// inform this livechan that we got a new post
func (lc *liveChan) Inform(post PostModel) {
	if lc.postchnl != nil {
		lc.postchnl <- post
	}
}

type httpFrontend struct {
	modui        ModUI
	httpmux      *mux.Router
	daemon       *NNTPDaemon
	cache        CacheInterface
	recvpostchan chan frontendPost
	bindaddr     string
	name         string

	secret string

	webroot_dir  string
	template_dir string
	static_dir   string

	regen_threads  int
	regen_on_start bool
	attachments    bool

	prefix          string
	regenThreadChan chan ArticleEntry
	regenGroupChan  chan groupRegenRequest

	store *sessions.CookieStore

	upgrader websocket.Upgrader

	jsonUsername string
	jsonPassword string
	enableJson   bool

	attachmentLimit int

	liveui_chnl       chan PostModel
	liveui_register   chan *liveChan
	liveui_deregister chan *liveChan
	end_liveui        chan bool
	// all liveui users
	// maps uuid -> liveChan
	liveui_chans map[string]*liveChan
}

// do we allow this newsgroup?
func (self httpFrontend) AllowNewsgroup(group string) bool {
	// XXX: hardcoded nntp prefix
	// TODO: make configurable nntp prefix
	return strings.HasPrefix(group, "overchan.") && newsgroupValidFormat(group) || group == "ctl" && group != "overchan."
}

func (self httpFrontend) PostsChan() chan frontendPost {
	return self.recvpostchan
}

func (self *httpFrontend) Regen(msg ArticleEntry) {
	self.cache.Regen(msg)
}

func (self httpFrontend) regenAll() {
	self.cache.RegenAll()
}

func (self *httpFrontend) regenerateBoard(group string) {
	self.cache.RegenerateBoard(group)
}

func (self httpFrontend) deleteThreadMarkup(root_post_id string) {
	self.cache.DeleteThreadMarkup(root_post_id)
}

func (self httpFrontend) deleteBoardMarkup(group string) {
	self.cache.DeleteBoardMarkup(group)
}

// load post model and inform live ui
func (self *httpFrontend) informLiveUI(msgid, group string) {
	model := self.daemon.database.GetPostModel(self.prefix, msgid)
	if model != nil && self.liveui_chnl != nil {
		self.liveui_chnl <- model
	}
}

// poll live ui events
func (self *httpFrontend) poll_liveui() {
	for {
		select {
		case live, ok := <-self.liveui_deregister:
			// deregister existing user event
			if ok {
				if self.liveui_chans != nil {
					delete(self.liveui_chans, live.uuid)
				}
				close(live.postchnl)
				live.postchnl = nil
			}
		case live, ok := <-self.liveui_register:
			// register new user event
			if ok {
				if self.liveui_chans != nil {
					live.uuid = randStr(10)
					live.postchnl = make(chan PostModel, 8)
					self.liveui_chans[live.uuid] = live
				}
				if live.resultchnl != nil {
					live.resultchnl <- live
				}
			}
		case model, ok := <-self.liveui_chnl:
			if ok {
				// inform global
				if ok {
					for _, livechan := range self.liveui_chans {
						livechan.Inform(model)
					}
				}
			}
		case <-self.end_liveui:
			livechnl := self.liveui_chnl
			self.liveui_chnl = nil
			close(livechnl)
			chnl := self.liveui_register
			self.liveui_register = nil
			close(chnl)
			chnl = self.liveui_deregister
			self.liveui_deregister = nil
			close(chnl)
			// remove all
			for _, livechan := range self.liveui_chans {
				if livechan.postchnl != nil {
					close(livechan.postchnl)
					livechan.postchnl = nil
				}
			}
			self.liveui_chans = nil
			return
		}
	}
}

func (self *httpFrontend) poll() {

	// regenerate front page
	self.cache.RegenFrontPage()

	// trigger regen
	if self.regen_on_start {
		self.cache.RegenAll()
	}

	modChnl := self.modui.MessageChan()
	for {
		select {
		case nntp := <-modChnl:
			f := self.daemon.store.CreateFile(nntp.MessageID())
			if f != nil {
				b := new(bytes.Buffer)
				err := nntp.WriteTo(b)
				if err == nil {
					r := bufio.NewReader(b)
					var hdr textproto.MIMEHeader
					hdr, err = readMIMEHeader(r)
					if err == nil {
						err = writeMIMEHeader(f, hdr)
						if err == nil {
							err = self.daemon.store.ProcessMessageBody(f, hdr, r)
						}
					}
				}
				f.Close()
				if err == nil {
					self.daemon.loadFromInfeed(nntp.MessageID())
				} else {
					log.Println("error storing mod message", err)
					DelFile(self.daemon.store.GetFilename(nntp.MessageID()))
				}
			} else {
				log.Println("failed to register mod message, file was not opened")
			}
		case nntp := <-self.recvpostchan:
			// get root post and tell frontend to regen that thread
			msgid := nntp.MessageID()
			group := nntp.Newsgroup()
			updateLinkCache()
			self.informLiveUI(msgid, group)
			if len(nntp.Reference()) > 0 {
				msgid = nntp.Reference()
			}
			entry := ArticleEntry{msgid, group}
			// regnerate thread
			self.regenThreadChan <- entry
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

// handle newboard page
func (self *httpFrontend) handle_newboard(wr http.ResponseWriter, r *http.Request) {
	param := make(map[string]interface{})
	param["prefix"] = self.prefix
	io.WriteString(wr, template.renderTemplate("newboard.mustache", param))
}

// handle new post via http request for a board
func (self *httpFrontend) handle_postform(wr http.ResponseWriter, r *http.Request, board string) {

	// the post we will turn into an nntp article
	var pr postRequest

	// do we send json reply?
	sendJson := r.URL.Query().Get("t") == "json"

	if sendJson {
		wr.Header().Add("Content-Type", "text/json; encoding=UTF-8")
	}

	// close request body when done
	defer r.Body.Close()

	mp_reader, err := r.MultipartReader()

	if err != nil {
		wr.WriteHeader(500)
		if sendJson {
			json.NewEncoder(wr).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			io.WriteString(wr, err.Error())
		}
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
	var url string
	url = fmt.Sprintf("%s-0.html", board)
	var part_buff bytes.Buffer
	for {
		part, err := mp_reader.NextPart()
		if err == nil {
			defer part.Close()
			// get the name of the part
			partname := part.FormName()
			// read part for attachment
			if strings.HasPrefix(partname, "attachment_") && self.attachments {
				if len(pr.Attachments) < self.attachmentLimit {
					// TODO: we could just write to disk the attachment so we're not filling ram up with crap
					att := readAttachmentFromMimePartAndStore(part, nil)
					if att != nil {
						log.Println("attaching file...")
						pr.Attachments = append(pr.Attachments, postAttachment{
							Filedata: att.Filedata(),
							Filename: att.Filename(),
							Filetype: att.Mime(),
						})
					}
				}
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
					url = fmt.Sprintf("thread-%s.html", HashMessageID(pr.Reference))
				}
			} else if partname == "captcha_id" {
				captcha_id = part_buff.String()
			} else if partname == "captcha" {
				captcha_solution = part_buff.String()
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
				if sendJson {
					json.NewEncoder(wr).Encode(map[string]interface{}{"error": errmsg})
				} else {
					io.WriteString(wr, errmsg)
				}
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
	resp_map := make(map[string]interface{})
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

	if captcha_retry {
		if sendJson {
			json.NewEncoder(wr).Encode(map[string]interface{}{"error": "bad captcha"})
		} else {
			// retry the post with a new captcha
			resp_map = make(map[string]interface{})
			resp_map["prefix"] = self.prefix
			resp_map["redirect_url"] = self.prefix + url
			resp_map["reason"] = "captcha incorrect"
			io.WriteString(wr, template.renderTemplate("post_fail.mustache", resp_map))
		}
		return
	}

	b := func() {
		if sendJson {
			wr.WriteHeader(200)
			json.NewEncoder(wr).Encode(map[string]interface{}{"error": "banned"})
		} else {
			wr.WriteHeader(403)
			io.WriteString(wr, "banned")
		}
	}

	e := func(err error) {
		wr.WriteHeader(200)
		if sendJson {
			json.NewEncoder(wr).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			resp_map["reason"] = err.Error()
			resp_map["prefix"] = self.prefix
			resp_map["redirect_url"] = self.prefix + url
			io.WriteString(wr, template.renderTemplate("post_fail.mustache", resp_map))
		}
	}

	s := func(nntp NNTPMessage) {
		// send success reply
		wr.WriteHeader(200)
		// determine the root post so we can redirect to the thread for it
		msg_id := nntp.Headers().Get("References", nntp.MessageID())
		// render response as success
		url := fmt.Sprintf("%sthread-%s.html", self.prefix, HashMessageID(msg_id))
		if sendJson {
			json.NewEncoder(wr).Encode(map[string]interface{}{"message_id": nntp.MessageID(), "url": url, "error": nil})
		} else {
			io.WriteString(wr, template.renderTemplate("post_success.mustache", map[string]interface{}{"prefix": self.prefix, "message_id": nntp.MessageID(), "redirect_url": url}))
		}
	}
	self.handle_postRequest(&pr, b, e, s, false)
}

// turn a post request into an nntp article write it to temp dir and tell daemon
func (self *httpFrontend) handle_postRequest(pr *postRequest, b bannedFunc, e errorFunc, s successFunc, createGroup bool) {
	var err error
	if len(pr.Attachments) > self.attachmentLimit {
		err = errors.New("too many attachments")
		e(err)
		return
	}
	nntp := new(nntpArticle)
	defer nntp.Reset()
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

	if !createGroup && !self.daemon.database.HasNewsgroup(board) {
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
	if len(pr.Attachments) == 0 && len(pr.Message) == 0 {
		e(errors.New("no message"))
		return
	}
	// TODO: make configurable
	if len(pr.Message) > 1024*1024 {
		e(errors.New("your message is too big"))
		return
	}

	if len(pr.Frontend) == 0 {
		// :-DDD
		pr.Frontend = "mongo.db.is.web.scale"
	} else if len(pr.Frontend) > 128 {
		e(errors.New("frontend name is too long"))
		return
	}

	subject := pr.Subject

	// set subject
	if len(subject) == 0 {
		subject = "None"
	} else if len(subject) > 256 {
		// subject too big
		e(errors.New("Subject is too long"))
		return
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
	if len(name) > 128 {
		// name too long
		e(errors.New("name too long"))
		return
	}
	msgid := genMessageID(pr.Frontend)
	// roll until dubs if desired
	for pr.Dubs && !MessageIDWillDoDubs(msgid) {
		msgid = genMessageID(pr.Frontend)
	}

	nntp.headers.Set("From", nntpSanitize(fmt.Sprintf("%s <poster@%s>", name, pr.Frontend)))
	nntp.headers.Set("Message-ID", msgid)

	// set message
	nntp.message = createPlaintextAttachment([]byte(pr.Message))
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
	if self.attachments {
		var delfiles []string
		for _, att := range pr.Attachments {
			// add attachment
			if len(att.Filedata) > 0 {
				a := createAttachment(att.Filetype, att.Filename, strings.NewReader(att.Filedata))
				nntp.Attach(a)
				err = a.Save(self.daemon.store.AttachmentDir())
				if err == nil {
					delfiles = append(delfiles, a.Filepath())
					err = self.daemon.store.GenerateThumbnail(a.Filepath())
					if err == nil {
						delfiles = append(delfiles, self.daemon.store.ThumbnailFilepath(a.Filepath()))
					}
				}
				if err != nil {
					break
				}
			}
		}
		if err != nil {
			// nuke files that
			for _, fname := range delfiles {
				DelFile(fname)
			}
			e(err)
			return
		}
		// pack it before sending so that the article is well formed
	}
	nntp.Pack()
	// sign if needed
	if len(tripcode_privkey) == nacl.CryptoSignSeedLen() {
		err = self.daemon.store.RegisterPost(nntp)
		if err != nil {
			e(err)
			return
		}
		nntp, err = signArticle(nntp, tripcode_privkey)
		if err != nil {
			// error signing
			e(err)
			return
		}
		if err == nil {
			err = self.daemon.store.RegisterSigned(nntp.MessageID(), nntp.Pubkey())
		}
	} else {
		err = self.daemon.store.RegisterPost(nntp)
	}
	if err != nil {
		e(err)
		return
	}
	// have daemon sign message
	// DON'T Wrap sign yet
	// wrapped := self.daemon.WrapSign(nntp)
	// save it
	f := self.daemon.store.CreateFile(nntp.MessageID())
	if f == nil {
		e(errors.New("failed to store article"))
		return
	} else {
		err = nntp.WriteTo(f)
		f.Close()
		if err == nil {
			self.daemon.loadFromInfeed(nntp.MessageID())
			s(nntp)
			return
		}
		// clean up
		DelFile(self.daemon.store.GetFilename(nntp.MessageID()))
		e(err)
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
	if r.Method == "POST" && self.AllowNewsgroup(board) && newsgroupValidFormat(board) && board != "ctl" {
		self.handle_postform(wr, r, board)
	} else {
		wr.WriteHeader(403)
		io.WriteString(wr, "Nope")
	}
}

func (self *httpFrontend) new_captcha(wr http.ResponseWriter, r *http.Request) {
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
		wr.Header().Add("Content-Type", "text/json; encoding=UTF-8")
		resp := make(map[string]string)
		resp["id"] = nntp.MessageID()
		enc := json.NewEncoder(wr)
		enc.Encode(resp)
	}

	dec := json.NewDecoder(r.Body)
	if api == "post" {
		var pr postRequest
		err = dec.Decode(&pr)
		r.Body.Close()
		if err == nil {
			// we parsed it
			self.handle_postRequest(&pr, b, e, s, true)
		} else {
			// bad parsing?
			api_error(wr, err)
		}
	} else {
		// no such method
		wr.WriteHeader(404)
		io.WriteString(wr, "No such method")
	}
}

// handle find post api command
func (self *httpFrontend) handle_api_find(wr http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	h := q.Get("hash")
	msgid := q.Get("id")
	if len(h) > 0 {
		e, err := self.daemon.database.GetMessageIDByHash(h)
		if err == nil {
			msgid = e.MessageID()
		}
	}
	if len(msgid) > 0 {
		// found it (probaly)
		model := self.daemon.database.GetPostModel(self.prefix, msgid)
		if model == nil {
			// no model
			wr.WriteHeader(404)
		} else {
			// we found it
			wr.Header().Add("Content-Type", "text/json; encoding=UTF-8")
			json.NewEncoder(wr).Encode(model)
		}
	} else {
		// not found
		wr.WriteHeader(404)
	}
}

// handle un authenticated part of api
func (self *httpFrontend) handle_unauthed_api(wr http.ResponseWriter, r *http.Request, api string) {
	var err error
	if api == "header" {
		var msgids []string
		q := r.URL.Query()
		name := q.Get("name")
		val := q.Get("value")
		msgids, err = self.daemon.database.GetMessageIDByHeader(name, val)
		if err == nil {
			wr.Header().Add("Content-Type", "text/json; encoding=UTF-8")
			json.NewEncoder(wr).Encode(msgids)
		} else {
			api_error(wr, err)
		}
	} else if api == "groups" {
		wr.Header().Add("Content-Type", "text/json; encoding=UTF-8")
		groups := self.daemon.database.GetAllNewsgroups()
		json.NewEncoder(wr).Encode(groups)
	} else if api == "find" {
		self.handle_api_find(wr, r)
	}
}

func (self *httpFrontend) handle_api(wr http.ResponseWriter, r *http.Request) {

	vars := mux.Vars(r)
	meth := vars["meth"]
	if r.Method == "POST" && self.enableJson {
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
		wr.WriteHeader(404)
	}
}

// upgrade to web sockets and subscribe to all new posts
// XXX: firehose?
func (self *httpFrontend) handle_liveui(w http.ResponseWriter, r *http.Request) {
	conn, err := self.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// problem
		w.WriteHeader(500)
		io.WriteString(w, err.Error())
		return
	}
	// obtain a new channel for reading post models
	livechnl := self.subscribeAll()
	if livechnl == nil {
		// shutting down
		conn.Close()
		return
	}
	// okay we got a channel
	live := <-livechnl
	close(livechnl)
	go func() {
		// read loop
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				conn.Close()
				return
			}
		}
	}()
	ticker := time.NewTicker(time.Second * 5)
	for err == nil {
		select {
		case model, ok := <-live.postchnl:
			if ok && model != nil {
				err = conn.WriteJSON(model)
			} else {
				// channel closed
				break
			}
		case <-ticker.C:
			conn.WriteMessage(websocket.PingMessage, []byte{})
		}
	}
	conn.Close()
	// deregister connection
	if self.liveui_deregister != nil {
		self.liveui_deregister <- live
	}
}

// get a chan that is subscribed to all new posts
func (self *httpFrontend) subscribeAll() chan *liveChan {
	if self.liveui_register == nil {
		return nil
	} else {
		live := new(liveChan)
		live.resultchnl = make(chan *liveChan)
		self.liveui_register <- live
		return live.resultchnl
	}
}

func (self *httpFrontend) Mainloop() {
	EnsureDir(self.webroot_dir)
	if !CheckFile(self.template_dir) {
		log.Fatalf("no such template folder %s", self.template_dir)
	}
	template.changeTemplateDir(self.template_dir)

	// set up handler mux
	self.httpmux = mux.NewRouter()

	self.httpmux.NotFoundHandler = template.createNotFoundHandler(self.prefix, self.name)

	// create mod ui
	self.modui = createHttpModUI(self)

	cache_handler := self.cache.GetHandler()

	// csrf protection
	b := []byte(self.secret)
	var sec [32]byte
	copy(sec[:], b)
	// TODO: make configurable
	CSRF := csrf.Protect(sec[:], csrf.Secure(false))

	m := mux.NewRouter()
	// modui handlers
	m.Path("/mod/").HandlerFunc(self.modui.ServeModPage).Methods("GET")
	m.Path("/mod/feeds").HandlerFunc(self.modui.ServeModPage).Methods("GET")
	m.Path("/mod/keygen").HandlerFunc(self.modui.HandleKeyGen).Methods("GET")
	m.Path("/mod/login").HandlerFunc(self.modui.HandleLogin).Methods("POST")
	m.Path("/mod/del/{article_hash}").HandlerFunc(self.modui.HandleDeletePost).Methods("GET")
	m.Path("/mod/ban/{address}").HandlerFunc(self.modui.HandleBanAddress).Methods("GET")
	m.Path("/mod/unban/{address}").HandlerFunc(self.modui.HandleUnbanAddress).Methods("GET")
	m.Path("/mod/addkey/{pubkey}").HandlerFunc(self.modui.HandleAddPubkey).Methods("GET")
	m.Path("/mod/delkey/{pubkey}").HandlerFunc(self.modui.HandleDelPubkey).Methods("GET")
	m.Path("/mod/admin/{action}").HandlerFunc(self.modui.HandleAdminCommand).Methods("GET", "POST")
	self.httpmux.PathPrefix("/mod/").Handler(CSRF(m))
	m = self.httpmux
	m.Path("/").Handler(cache_handler)
	// robots.txt handler
	m.Path("/robots.txt").HandlerFunc(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "User-Agent: *\nDisallow: /\n")
	})).Methods("GET")

	m.Path("/thm/{f}").Handler(http.FileServer(http.Dir(self.webroot_dir)))
	m.Path("/img/{f}").Handler(http.FileServer(http.Dir(self.webroot_dir)))
	m.Path("/{f}.html").Handler(cache_handler).Methods("GET", "HEAD")
	m.Path("/{f}.json").Handler(cache_handler).Methods("GET", "HEAD")
	m.Path("/static/{f}").Handler(http.FileServer(http.Dir(self.static_dir)))
	m.Path("/post/{f}").HandlerFunc(self.handle_poster).Methods("POST")
	m.Path("/captcha/img").HandlerFunc(self.new_captcha).Methods("GET")
	m.Path("/captcha/{f}").Handler(captcha.Server(350, 175)).Methods("GET")
	m.Path("/captcha/new.json").HandlerFunc(self.new_captcha_json).Methods("GET")
	m.Path("/new/").HandlerFunc(self.handle_newboard).Methods("GET")
	m.Path("/api/{meth}").HandlerFunc(self.handle_api).Methods("POST", "GET")
	// live ui websocket
	m.Path("/live").HandlerFunc(self.handle_liveui).Methods("GET")
	// live ui page
	m.Path("/livechan/").HandlerFunc(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		template.writeTemplate("live.mustache", map[string]interface{}{"prefix": self.prefix}, w)
	})).Methods("GET", "HEAD")
	var err error

	// run daemon's mod engine with our frontend
	go RunModEngine(self.daemon.mod, self.cache.RegenOnModEvent)

	self.cache.Start()

	// poll channels
	go self.poll()

	// poll liveui
	go self.poll_liveui()

	// start webserver here
	log.Printf("frontend %s binding to %s", self.name, self.bindaddr)

	// serve it!
	err = http.ListenAndServe(self.bindaddr, self.httpmux)
	if err != nil {
		log.Fatalf("failed to bind frontend %s %s", self.name, err)
	}
}

func (self *httpFrontend) endLiveUI() {
	// end live ui
	if self.end_liveui != nil {
		self.end_liveui <- true
		close(self.end_liveui)
		self.end_liveui = nil
	}
}

// create a new http based frontend
func NewHTTPFrontend(daemon *NNTPDaemon, cache CacheInterface, config map[string]string, url string) Frontend {
	front := new(httpFrontend)
	front.daemon = daemon
	front.cache = cache
	front.attachments = mapGetInt(config, "allow_files", 1) == 1
	front.bindaddr = config["bind"]
	front.name = config["name"]
	front.webroot_dir = config["webroot"]
	front.static_dir = config["static_files"]
	front.template_dir = config["templates"]
	front.prefix = config["prefix"]
	front.regen_on_start = config["regen_on_start"] == "1"
	if config["json-api"] == "1" {
		front.jsonUsername = config["json-api-username"]
		front.jsonPassword = config["json-api-password"]
		front.enableJson = true
	}
	front.attachmentLimit = 5
	front.secret = config["api-secret"]
	front.store = sessions.NewCookieStore([]byte(front.secret))
	front.store.Options = &sessions.Options{
		// TODO: detect http:// etc in prefix
		Path:   front.prefix,
		MaxAge: 10000000, // big number
	}
	front.recvpostchan = make(chan frontendPost)
	front.regenThreadChan = front.cache.GetThreadChan()
	front.regenGroupChan = front.cache.GetGroupChan()

	// liveui related members
	front.liveui_chnl = make(chan PostModel, 128)
	front.liveui_register = make(chan *liveChan)
	front.liveui_deregister = make(chan *liveChan)
	front.liveui_chans = make(map[string]*liveChan)
	front.end_liveui = make(chan bool)
	return front
}
