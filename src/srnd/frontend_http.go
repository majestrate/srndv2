//
// frontend_http.go
//
// srnd http frontend implementation
//
package srnd

import (
  "github.com/dchest/captcha"
  "github.com/gorilla/mux"
  "github.com/gorilla/sessions"
  "github.com/gorilla/websocket"
  "github.com/majestrate/srndv2/src/nacl"
  "bytes"
  "encoding/json"
  "fmt"
  "io"
  "log"
  "net"
  "net/http"
  "os"
  "path/filepath"
  "strings"
  "time"
)


// regenerate a newsgroup page
type groupRegenRequest struct {
  // which newsgroup
  group string
  // page number
  page int
}

type httpFrontend struct {

  modui ModUI
  httpmux *mux.Router
  daemon *NNTPDaemon
  postchan chan NNTPMessage
  recvpostchan chan NNTPMessage
  bindaddr string
  name string

  webroot_dir string
  template_dir string
  static_dir string
  liveui_static_dir string
  
  regen_threads int
  regen_on_start bool
  attachments bool
  
  prefix string
  regenThreadChan chan string
  regenGroupChan chan groupRegenRequest
  ukkoChan chan bool
  regenBoard map[string]groupRegenRequest
  
  regenBoardTicker *time.Ticker
  
  store *sessions.CookieStore

  upgrader websocket.Upgrader
}

// do we allow this newsgroup?
func (self httpFrontend) AllowNewsgroup(group string) bool {
  // XXX: hardcoded nntp prefix
  // TODO: make configurable nntp prefix
  return strings.HasPrefix(group, "overchan.") && newsgroupValidFormat(group) || group == "ctl"
}

// try to delete root post's page
func (self httpFrontend) deleteThreadMarkup(root_post_id string) {
  fname :=  self.getFilenameForThread(root_post_id)
  log.Println("delete file", fname)
  os.Remove(fname)
}

func (self httpFrontend) getFilenameForThread(root_post_id string) string {
  fname := fmt.Sprintf("thread-%s.html", ShortHashMessageID(root_post_id))
  return filepath.Join(self.webroot_dir, fname)
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
      self.regenerateBoard(group)
    }
  }
}


type boardPageRow struct {
  Board string
  Hour int64 
  Day int64 
  All int64 
}

type boardPageRows []boardPageRow

func (self boardPageRows) Len() int {
  return len(self)
}

func (self boardPageRows) Less(i, j int) bool {
  i_val := self[i]
  j_val := self[j]
  return (i_val.Day + i_val.Hour * 24 ) > ( j_val.Day + j_val.Hour * 24)
}

func (self boardPageRows) Swap(i, j int) {
  self[i] , self[j] = self[j], self[i]
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
      // listen for regen thread requests
    case msgid := <- self.regenThreadChan:
      self.regenerateThread(msgid)
      // listen for regen board requests
    case req := <- self.regenGroupChan:
      self.regenBoard[fmt.Sprintf("%s|%s", req.group, req.page)] = req
    case nntp := <- modChnl:
      // forward signed messages to daemon
      self.postchan <- nntp
    case nntp := <- self.recvpostchan:
      // get root post and tell frontend to regen that thread
      var msgid string
      if len(nntp.Reference()) > 0 {
        msgid = nntp.Reference()
      } else {
        msgid = nntp.MessageID()
      }
      self.regenerateThread(msgid)
      // regen the newsgroup we're in
      // TODO: regen only what we need to
      pages := self.daemon.database.GetGroupPageCount(nntp.Newsgroup())
      // regen all pages
      var page int64
      group := nntp.Newsgroup()
      for ; page < pages ; page ++ {
        req := groupRegenRequest{
          group: group,
          page: int(page),
        }
        self.regenBoard[fmt.Sprintf("%s|%s", req.group, req.page)] = req
      }
      // regen ukko
    case regen_front := <- self.ukkoChan:
      self.regenUkko()
      if regen_front {
        self.regenFrontPage()
      }
    case _ = <- self.regenBoardTicker.C:
      for _, v := range self.regenBoard {
        self.regenerateBoardPage(v.group, v.page)
      }
      self.regenBoard = make(map[string]groupRegenRequest)
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
  enc.Encode(resp)
}

// handle a request to the websocket ui
func (self httpFrontend) handle_liveui(wr http.ResponseWriter, r *http.Request) {
  ws, err := self.upgrader.Upgrade(wr, r, nil)
  if err == nil {
    // we upgraded fine
    // make a connection
    //conn := createConnection(ws)

    // reader loop
    go func() {
      for {

          ws.Close()
          return
      }
    }()
    
    for {
      select {
      //case json_data := <- conn.to_daemon_chan:
      //case msg := <- conn.from_daemon_chan:
      //  err := ws.WriteJSON(msg)
        
      }
    }
  }
}


// write out a json object of the liveui's options
// this includes site prefix so the js ui knows where it is
func (self httpFrontend) handle_liveui_options(wr http.ResponseWriter, r *http.Request) {
  resp := make(map[string]string)
  resp["prefix"] = self.prefix
  enc := json.NewEncoder(wr)
  enc.Encode(resp)
}

func (self httpFrontend) handle_liveui_index(wr http.ResponseWriter, r *http.Request) {
  io.WriteString(wr, template.renderTemplate("live.mustache", map[string]string{ "prefix" : self.prefix }))
}

// regen every page of the board
func (self httpFrontend) regenerateBoard(group string) {
  template.genBoard(self.prefix, self.name,  group, self.webroot_dir, self.daemon.database)
}

// regenerate just a thread page
func (self httpFrontend) regenerateThread(msgid string) {
  fname := self.getFilenameForThread(msgid)
  template.genThread(msgid, self.prefix, self.name, fname, self.daemon.database)
}

// regenerate just a page on a board
func (self httpFrontend) regenerateBoardPage(board string, page int) {
  fname := self.getFilenameForBoardPage(board, page)
  template.genBoardPage(self.prefix, self.name, board, page, fname, self.daemon.database)
}

// regenerate the front page
func (self httpFrontend) regenFrontPage() {
  fname := filepath.Join(self.webroot_dir, "index.html")
  template.genFrontPage(10, self.name, fname, self.daemon.database)
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
    self.regenThreadChan <- root
  }
  self.regenGroupChan <- groupRegenRequest{newsgroup, int(page)}
}


// handle new post via http request for a board
func (self httpFrontend) handle_postform(wr http.ResponseWriter, r *http.Request, board string) {

  // always lower case newsgroups
  board = strings.ToLower(board)
  
  // post fail message
  post_fail := ""
  captcha_solved := false

  // post message
  msg := ""
  
  // the nntp message
  var nntp nntpArticle
  nntp.headers = make(ArticleHeaders)


  // tripcode private key
  var tripcode_privkey []byte



  // encrypt IP Addresses
  // when a post is recv'd from a frontend, the remote address is given its own symetric key that the local srnd uses to encrypt the address with, for privacy
  // when a mod event is fired, it includes the encrypted IP address and the symetric key that frontend used to encrypt it, thus allowing others to determine the IP address
  // each stnf will optinally comply with the mod event, banning the address from being able to post from that frontend
  // this will be done eventually but for now that requires too much infrastrucutre, let's go with regular IP Addresses for now.
  
  // get the "real" ip address from the request

  address , _, err := net.SplitHostPort(r.RemoteAddr)
  // TODO: have in config upstream proxy ip and check for that
  if strings.HasPrefix(address, "127.") {
    // if it's loopback check headers for reverse proxy headers
    // TODO: make sure this isn't a tor user being sneaky
    address = getRealIP(r.Header.Get("X-Real-IP"))
  }
    
  // check for banned
  if len(address) > 0 {
    banned, err :=  self.daemon.database.CheckIPBanned(address)
    if err == nil {
      if banned {
        wr.WriteHeader(403)
        // TODO: ban messages
        io.WriteString(wr,  "nigguh u banned.")
        return
      }
    } else {
      wr.WriteHeader(500)
      io.WriteString(wr, "error checking for ban: ")
      io.WriteString(wr, err.Error())
      return
    }
  }
  if len(address) == 0 {
    address = "Tor"
  }
  if ! strings.HasPrefix(address, "127.") {
    // set the ip address of the poster to be put into article headers
    // if we cannot determine it, i.e. we are on Tor/i2p, this value is not set
    if address == "Tor" {
      nntp.headers.Set("X-Tor-Poster", "1")
    } else {
      address, err = self.daemon.database.GetEncAddress(address)
      nntp.headers.Set("X-Encrypted-IP", address)
      // TODO: add x-tor-poster header for tor exits
    }
  }
  
  // if we don't have an address for the poster try checking for i2p httpd headers
  address = r.Header.Get("X-I2P-DestHash")
  // TODO: make sure this isn't a Tor user being sneaky
  if len(address) > 0 {
    nntp.headers.Set("X-I2P-DestHash", address)
  }
  

  // set newsgroup
  nntp.headers.Set("Newsgroups", board)
  
  // redirect url
  url := self.prefix
  // mime part handler
  var part_buff bytes.Buffer
  mp_reader, err := r.MultipartReader()
  if err != nil {
    errmsg := fmt.Sprintf("httpfrontend post handler parse multipart POST failed: %s", err)
    log.Println(errmsg)
    wr.WriteHeader(500)
    io.WriteString(wr, errmsg)
    return
  }
  for {
    part, err := mp_reader.NextPart()
    if err == nil {
      // get the name of the part
      partname := part.FormName()

      // read part for attachment
      if partname == "attachment" && self.attachments {
        log.Println("attaching file...")
        att := readAttachmentFromMimePart(part)
        nntp = nntp.Attach(att).(nntpArticle)
        continue
      }

      io.Copy(&part_buff, part)
      
      // check for values we want
      if partname == "subject" {
        subject := part_buff.String()
        if len(subject) == 0 {
          subject = "None"
        }
        nntp.headers.Set("Subject", subject)
        if isSage(subject) {
          nntp.headers.Set("X-Sage", "1")
        }
      } else if partname == "name" {
        name := part_buff.String()
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
        nntp.headers.Set("From", nntpSanitize(fmt.Sprintf("%s <%s@%s>", name, name, self.name)))
        nntp.headers.Set("Message-ID", genMessageID(self.name))
      } else if partname == "message" {
        msg = part_buff.String()
      } else if partname == "reference" {
        ref := part_buff.String()
        if len(ref) == 0 {
          url = fmt.Sprintf("%s.html", board)
        } else if ValidMessageID(ref) {
          if self.daemon.database.HasArticleLocal(ref) {
            nntp.headers.Set("References", ref)
            url = fmt.Sprintf("thread-%s.html", ShortHashMessageID(ref))
          } else {
            // no such article
            url = fmt.Sprintf("%s.html", board)
            post_fail += "we don't have "
            post_fail += ref
            post_fail += "locally, can't reply. "
          }
        } else {
          post_fail += "invalid reference: "
          post_fail += ref
          post_fail += ", not posting. "
        }
          

      } else if partname == "captcha" {
        captcha_solution := part_buff.String()
        s, err := self.store.Get(r, self.name)
        captcha_id , ok := s.Values["captcha_id"]
        if err == nil && ok {
          if captcha.VerifyString(captcha_id.(string), captcha_solution) {
            // captcha is valid
            captcha_solved = true
          } else {
            // captcha is not valid
            post_fail += "failed captcha. "
          }
        } else {
          post_fail += "enable cookies. "
        }
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


  // make error template param
  resp_map := make(map[string]string)
  resp_map["redirect_url"] = self.prefix + url

  if len(nntp.attachments) == 0 && len(msg) == 0 {
    post_fail += "no message. "
  }

  if ! captcha_solved {
    post_fail += "no captcha."
  }
  
  if len(post_fail) > 0 {
    wr.WriteHeader(200)
    resp_map["reason"] = post_fail
    io.WriteString(wr, template.renderTemplate("post_fail.mustache", resp_map))
    return
  }
  // set message
  nntp.message = createPlaintextAttachment(msg)
  // set date
  nntp.headers.Set("Date", timeNowStr())
  // append path from frontend
  nntp.AppendPath(self.name)
  // send message off to daemon
  log.Printf("uploaded %d attachments", len(nntp.Attachments()))
  nntp.Pack()

  // sign if needed
  if len(tripcode_privkey) == nacl.CryptoSignSecretLen() {
    nntp, err = signArticle(nntp, tripcode_privkey)
    if err != nil {
      // wtf? error!?
      log.Println("error signing", err)
      wr.WriteHeader(500)
      io.WriteString(wr, err.Error())
      return 
    }
  }
  // XXX: write it temp instead
  // self.postchan <- nntp
  f := self.daemon.store.CreateTempFile(nntp.MessageID())
  if f != nil {
    nntp.WriteTo(f, "\n")
    f.Close()
  }
  self.daemon.infeed_load <- nntp.MessageID()

  // send success reply
  wr.WriteHeader(200)
  // determine the root post so we can redirect to the thread for it
  msg_id := nntp.headers.Get("References", nntp.MessageID())
  // render response as success
  url = fmt.Sprintf("%sthread-%s.html", self.prefix, ShortHashMessageID(msg_id))
  io.WriteString(wr, template.renderTemplate("post_success.mustache", map[string]string {"message_id" : nntp.MessageID(), "redirect_url" : url}))
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
  s , err := self.store.Get(r, self.name)
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

func (self httpFrontend) Mainloop() {
  EnsureDir(self.webroot_dir)
  if ! CheckFile(self.template_dir) {
    log.Fatalf("no such template folder %s", self.template_dir)
  }

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
  // liveui handlers

  self.httpmux.Path("/live/").HandlerFunc(self.handle_liveui_index).Methods("GET")
  self.httpmux.Path("/live/options").HandlerFunc(self.handle_liveui_options).Methods("GET")
  self.httpmux.Path("/live/ws").HandlerFunc(self.handle_liveui).Methods("GET")

  var err error

  // poll channels
  go self.poll()
  // regen ukko / front page
  tick := time.NewTicker(time.Second * 30)
  chnl := tick.C
  go func() {
    t := <- chnl
    self.ukkoChan <- t.Minute() == 0 && t.Second() < 30
  }()

  go RunModEngine(self.daemon.mod, self.regenOnModEvent)
  
  // start webserver here
  log.Printf("frontend %s binding to %s", self.name, self.bindaddr)
  
  err = http.ListenAndServe(self.bindaddr, self.httpmux)
  if err != nil {
    log.Fatalf("failed to bind frontend %s %s", self.name, err)
  }
}

func (self httpFrontend) Regen(msg ArticleEntry) {
  self.regenThreadChan <- msg.MessageID()
  self.regenerateBoard(msg.Newsgroup())
  self.ukkoChan <- true
}


// create a new http based frontend
func NewHTTPFrontend(daemon *NNTPDaemon, config map[string]string, url string) Frontend {
  var front httpFrontend
  front.daemon = daemon
  front.regenBoardTicker = time.NewTicker(time.Second * 10)
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
  front.store = sessions.NewCookieStore([]byte(config["api-secret"]))
  front.store.Options = &sessions.Options{
    // TODO: detect http:// etc in prefix
    Path: front.prefix,
    MaxAge: 600,
  }
  front.postchan = make(chan NNTPMessage, 16)
  front.recvpostchan = make(chan NNTPMessage, 16)
  front.regenThreadChan = make(chan string, 16)
  front.regenGroupChan = make(chan groupRegenRequest, 8)
  front.ukkoChan = make(chan bool)
  return front
}
