//
// frontend_http.go
//
// srnd http frontend implementation
//
package srnd

import (
  "github.com/dchest/captcha"
  "bytes"
  "fmt"
  "io"
  "log"
  "net"
  "net/http"
  "path/filepath"
  "strings"
)

type groupRegenRequest struct {
  group string
  page int
}

type httpFrontend struct {

  modui ModUI
  httpmux *http.ServeMux
  daemon *NNTPDaemon
  postchan chan *NNTPMessage
  recvpostchan chan *NNTPMessage
  bindaddr string
  name string

  webroot_dir string
  template_dir string

  prefix string
  regenThreadChan chan string
  regenGroupChan chan groupRegenRequest
}

func (self httpFrontend) AllowNewsgroup(group string) bool {
  return strings.HasPrefix(group, "overchan.")
}


func (self httpFrontend) getFilenameForThread(root_post_id string) string {
  fname := fmt.Sprintf("thread-%s.html", ShortHashMessageID(root_post_id))
  return filepath.Join(self.webroot_dir, fname)
}

func (self httpFrontend) getFilenameForBoardPage(boardname string, pageno int) string {
  fname := fmt.Sprintf("%s-%d.html", boardname, pageno)
  return filepath.Join(self.webroot_dir, fname)
}

func (self httpFrontend) NewPostsChan() chan *NNTPMessage {
  return self.postchan
}

func (self httpFrontend) PostsChan() chan *NNTPMessage {
  return self.recvpostchan
}

func (self httpFrontend) loghttp(req *http.Request, code int) {
  log.Printf("%s -- %s %s -- %d", self.name, req.Method, req.URL.Path, code)
}


// regen every newsgroup
func (self httpFrontend) regenAll() {
  log.Println("regen all on http frontend")
  // get all groups
  groups := self.daemon.database.GetAllNewsgroups()
  if groups != nil {
    for _, group := range groups {
      // send every thread for this group down the regen thread channel
      self.daemon.database.GetGroupThreads(group, self.regenThreadChan)
      // regen the entire board too
      pages := self.daemon.database.GetGroupPageCount(group)
      // regen all pages
      var page int64
      for ; page < pages ; page ++ {
        req := groupRegenRequest{group, int(page)}
        self.regenGroupChan <- req
      }

    }
  }
  self.regenUkko()
}


// regenerate a board page for newsgroup
func (self httpFrontend) regenerateBoard(newsgroup string, pageno int) {
  // TODO: hard coded threads per page
  board_page := self.daemon.database.GetGroupForPage(self.prefix, self.name, newsgroup, pageno, 10)
  if board_page == nil {
    log.Println("failed to regen board", newsgroup)
    return
  }
  fname := self.getFilenameForBoardPage(newsgroup, pageno)
  wr, err := OpenFileWriter(fname)
  if err == nil {
    err = board_page.RenderTo(wr)
    wr.Close()
    if err != nil {
      log.Println("did not write board page",fname, err)
    }
  } else {
    log.Println("cannot open", fname, err)
  }
  log.Println("regenerated file", fname)
}

// regenerate the ukko page
func (self httpFrontend) regenUkko() {
  // get the last 5 bumped threads
  roots := self.daemon.database.GetLastBumpedThreads("", 5)
  var threads []ThreadModel
  for _, rootpost := range roots {
    // for each root post
    // get the last 5 posts
    post := self.daemon.database.GetPostModel(self.prefix, rootpost)
    if post == nil {
      log.Println("failed to get root post", rootpost)
      return
    }
    posts := []PostModel{post}
    if self.daemon.database.ThreadHasReplies(rootpost) {
      repls := self.daemon.database.GetThreadReplyPostModels(self.prefix, rootpost, 5)
      if repls == nil {
        log.Println("failed to get replies for", rootpost)
        return
      }
      posts = append(posts, repls...)
    }
    threads = append(threads, NewThreadModel(self.prefix, posts))
  }
  wr, err := OpenFileWriter(filepath.Join(self.webroot_dir, "ukko.html"))
  if err == nil {
    io.WriteString(wr, renderUkko(self.prefix, threads))
    wr.Close()
  } else {
    log.Println("error generating ukko", err)
  }
}

// regnerate a thread given the messageID of the root post
// TODO: don't load from store
func (self httpFrontend) regenerateThread(rootMessageID string) {
  // get the root post
  post := self.daemon.database.GetPostModel(self.prefix, rootMessageID)
  if post == nil {
    log.Println("failed to regen thread, root post is nil", rootMessageID)
    return
  }
  posts := []PostModel{post}
  // get replies
  if self.daemon.database.ThreadHasReplies(rootMessageID) {
    repls :=  self.daemon.database.GetThreadReplyPostModels(self.prefix, rootMessageID, 0)
    if repls == nil {
      log.Println("failed to regen thread, replies was nil for op", rootMessageID)
      return
    }
    posts = append(posts, repls...)
  }
  
  // make thread model
  thread := NewThreadModel(self.prefix, posts)
  // get filename for thread
  fname := self.getFilenameForThread(rootMessageID)
  // open writer for file
  wr, err := OpenFileWriter(fname)
  if err != nil {
    log.Println(err)
    return
  }
  // render the thread
  err = thread.RenderTo(wr)
  wr.Close()
  if err == nil {
    log.Printf("regenerated file %s", fname)
  } else {
    log.Printf("failed to render %s", err)
  }  
}

func (self httpFrontend) poll() {
  chnl := self.PostsChan()
 
  for {
    select {
    case nntp := <- chnl:
      // get root post and tell frontend to regen that thread
      if len(nntp.Reference) > 0 {
        self.regenThreadChan <- nntp.Reference
      } else {
        self.regenThreadChan <- nntp.MessageID
      }
      // regen the newsgroup we're in
      // TODO: regen only what we need to
      pages := self.daemon.database.GetGroupPageCount(nntp.Newsgroup)
      // regen all pages
      var page int64
      for ; page < pages ; page ++ {
        req := groupRegenRequest{nntp.Newsgroup, int(page)}
        self.regenGroupChan <- req
      }
      // regen ukko
      self.regenUkko()
    }
  }
}

// select loop for channels
func (self httpFrontend) pollregen() {
  for {
    select {
      
      // listen for regen thread requests
    case msgid := <- self.regenThreadChan:
      self.regenerateThread(msgid)
      
      // listen for regen board requests
    case req := <- self.regenGroupChan:
      self.regenerateBoard(req.group, req.page)
    }
  }
}

func (self httpFrontend) handle_postform(wr http.ResponseWriter, r *http.Request, board string) {

  // default values
  // TODO: set content type for attachments
  content_type := "text/plain"
  ref := ""
  name := "anonymous"
  email := ""
  subject := "None"
  message := ""
  // captcha stuff
  captcha_id := ""
  captcha_solution := ""
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
      // we got a part
      // read the body first
      io.Copy(&part_buff, part)
      // get the name of the part
      partname := part.FormName()
      // check for values we want
      if partname == "email" {
        email = part_buff.String()
      } else if partname == "subject" {
        subject = part_buff.String()
      } else if partname == "name" {
        name = part_buff.String()
      } else if partname == "message" {
        message = part_buff.String()
      } else if partname == "reference" {
        ref = part_buff.String()
      } else if partname == "captcha" {
        captcha_id = part_buff.String()
      } else if partname == "captcha_solution" {
        captcha_solution = part_buff.String()
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

  url := self.prefix
  if len(ref) > 0 {
    // redirect to thread
    url += fmt.Sprintf("thread-%s.html", ShortHashMessageID(ref))
  } else {
    // redirect to board
    url += fmt.Sprintf("%s.html", board)
  }

  // make error template param
  resp_map := make(map[string]string)
  resp_map["redirect_url"] = url
  postfail := ""
  
  if len(captcha_solution) == 0 || len(captcha_id) == 0 {
    postfail = "no captcha provided"
  }
  if ! captcha.VerifyString(captcha_id, captcha_solution) {
    postfail = "failed captcha"
  }

  if len(message) == 0 {
    postfail = "message too small"
  }
  if len(postfail) > 0 {
    wr.WriteHeader(200)
    resp_map["reason"] = postfail
    fname := filepath.Join(defaultTemplateDir(), "post_fail.mustache")
    io.WriteString(wr, templateRender(fname, resp_map))
    return
  }
  
  // make the message
  nntp := new(NNTPMessage)
  // generate message id
  nntp.MessageID = fmt.Sprintf("<%s%d@%s>", randStr(12), timeNow(), self.name)
  // TODO: hardcoded newsgroup prefix
  nntp.Newsgroup = board
  if len(name) > 0 {
    nntp.Name = nntpSanitize(name)
  } else {
    nntp.Name = "Anonymous"
  }
  nntp.Email = nntp.Name + "@" + self.name
  if len(subject) > 0 {
    nntp.Subject = nntpSanitize(subject)
  } else {
    nntp.Subject = "None"
  }
  nntp.Path = self.name
  nntp.Posted = timeNow()
  nntp.Message = nntpSanitize(message)
  nntp.ContentType = content_type
  nntp.Sage = strings.HasPrefix(strings.ToLower(email), "sage")
  // set reference
  if ValidMessageID(ref) {
    nntp.Reference = ref
  }
  nntp.OP = len(ref) == 0

  // set ip address info
  // TODO:
  //  encrypt IP Addresses
  //  when a post is recv'd from a frontend, the remote address is given its own symetric key that the local srnd uses to encrypt the address with, for privacy
  //  when a mod event is fired, it includes the encrypted IP address and the symetric key that frontend used to encrypt it, thus allowing others to determine the IP address
  //  each stnf will optinally comply with the mod event, banning the address from being able to post from that frontend
  //  this will be done eventually but for now that requires too much infrastrucutre, let's go with regular IP Addresses for now.
  //
  nntp.ExtraHeaders = make(map[string]string)
  // get the "real" ip address from the request
  address := ""
  host, _, err := net.SplitHostPort(r.RemoteAddr)
  if err == nil {
    address = getRealIP(host)
  }
  if len(address) == 0 {
    // fall back to X-Real-IP header optinally set by reverse proxy
    // TODO: make sure this isn't a Tor user being sneaky
    address = getRealIP(r.Header.Get("X-Real-IP"))
  }
  if len(address) > 0 {
    // set the ip address of the poster to be put into article headers
    // if we cannot determine it, i.e. we are on Tor/i2p, this value is not set
    // nntp.ExtraHeaders["X-Encrypted-IP"] = address
  } else {
    // if we don't have an address for the poster try checking for i2p httpd headers
    address = r.Header.Get("X-I2P-DestHash")
    // TODO: make sure this isn't a Tor user being sneaky
    if len(address) > 0 {
      nntp.ExtraHeaders["X-Frontend-DestHash"] = address
    }
  }

  
  // send message off to daemon
  self.postchan <- nntp

  // send success reply
  wr.WriteHeader(200)
  msg_id := nntp.Reference
  if len(msg_id) == 0 {
    msg_id = nntp.MessageID
  }
  url = fmt.Sprintf("%sthread-%s.html", self.prefix, ShortHashMessageID(msg_id))
  fname := filepath.Join(defaultTemplateDir(), "post_success.mustache")
  io.WriteString(wr, templateRender(fname, map[string]string {"message_id" : nntp.MessageID, "redirect_url" : url}))
}



// handle posting / postform
func (self httpFrontend) handle_poster(wr http.ResponseWriter, r *http.Request) {
  path := r.URL.Path
  var board string
  // extract board
  if strings.Count(path, "/") > 1 {
    board = strings.Split(path,"/")[2]
  }
  // this is a POST request
  if r.Method == "POST" && strings.HasPrefix(board, "overchan.") && newsgroupValidFormat(board) {
    self.handle_postform(wr, r, board)
  } else {
      wr.WriteHeader(403)
      io.WriteString(wr, "Nope")
  }
}

func (self httpFrontend) new_captcha(wr http.ResponseWriter, r *http.Request) {
  io.WriteString(wr, captcha.New())
}

func (self httpFrontend) Mainloop() {
  EnsureDir(self.webroot_dir)
  if ! CheckFile(self.template_dir) {
    log.Fatalf("no such template folder %s", self.template_dir)
  }

  // regen threads
  go self.pollregen()
  // poll channels
  go self.poll()

  // trigger regen
  go self.regenAll()
  
  // start webserver here
  log.Printf("frontend %s binding to %s", self.name, self.bindaddr)
  // set up handler mux
  self.httpmux = http.NewServeMux()
  // register handlers for mux
  // webroot handler
  self.httpmux.Handle("/", http.FileServer(http.Dir(self.webroot_dir)))
  // post handler
  self.httpmux.HandleFunc("/post/", self.handle_poster)
  // captcha handlers
  self.httpmux.Handle("/captcha/", captcha.Server(350, 175))
  self.httpmux.HandleFunc("/captcha", self.new_captcha)
  // modui handlers
  self.httpmux.Handle("/mod/", self.modui)
  
  err := http.ListenAndServe(self.bindaddr, self.httpmux)
  if err != nil {
    log.Fatalf("failed to bind frontend %s %s", self.name, err)
  }
}


// create a new http based frontend
func NewHTTPFrontend(daemon *NNTPDaemon, config map[string]string) Frontend {
  var front httpFrontend
  front.daemon = daemon
  front.bindaddr = config["bind"]
  front.name = config["name"]
  front.webroot_dir = config["webroot"]
  front.template_dir = config["templates"]
  front.prefix = config["prefix"]
  front.postchan = make(chan *NNTPMessage, 16)
  front.recvpostchan = make(chan *NNTPMessage, 16)
  front.regenThreadChan = make(chan string, 16)
  front.regenGroupChan = make(chan groupRegenRequest, 8)
  return front
}
