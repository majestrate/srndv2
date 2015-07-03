//
// frontend.go
// srnd static html frontend
//
//
package srnd

import (
  "fmt"
  "io"
  "log"
  "net/http"
  "path/filepath"
  "strings"
)

// frontend interface for any type of frontend
type Frontend interface {

  // channel that is for the nntpd to poll for new posts from this frontend
  NewPostsChan() chan *NNTPMessage

  // channel that is for the frontend to pool for new posts from the nntpd
  PostsChan() chan *NNTPMessage
  
  // run mainloop
  Mainloop()

  // do we want posts from a newsgroup?
  AllowNewsgroup(group string) bool
  
}

// muxed frontend for holding many frontends
type multiFrontend struct {
  Frontend

  muxedpostchan chan *NNTPMessage
  muxedchan chan *NNTPMessage
  frontends []Frontend
}


func (self multiFrontend) Mainloop() {
  for idx := range(self.frontends) {
    go self.frontends[idx].Mainloop()
    go self.forwardPosts(self.frontends[idx])
  }
  

  // poll for incoming 
  chnl := self.PostsChan()
  for {
    select {
    case nntp := <- chnl:
      for _ , frontend := range self.frontends {
        ch := frontend.PostsChan()
        ch <- nntp
      }
      break
    }
  }
}

func (self multiFrontend) forwardPosts(front Frontend) {
  chnl := front.NewPostsChan()
  for {
    select {
    case nntp := <- chnl:
      // put in the path header the fact that this passed through the multifrontend
      // why? because why not.
      nntp.Path = "srndv2.frontend.mux!" + nntp.Path
      self.muxedpostchan <- nntp
    }
  }
}

func (self multiFrontend) NewPostsChan() chan *NNTPMessage {
  return self.muxedpostchan
}

func (self multiFrontend) PostsChan() chan *NNTPMessage {
  return self.muxedchan
}


func MuxFrontends(fronts ...Frontend) Frontend {
  var front multiFrontend
  front.muxedchan = make(chan *NNTPMessage, 64)
  front.muxedpostchan = make(chan *NNTPMessage, 64)
  front.frontends = fronts
  return front
}

type httpFrontend struct {
  Frontend

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
  regenGroupChan chan string
}

func (self httpFrontend) AllowNewsgroup(group string) bool {
  return strings.HasPrefix(group, "overchan.")
}


func (self httpFrontend) getFilenameForThread(root_post_id string) string {
  fname := fmt.Sprintf("thread-%s.html", ShortHashMessageID(root_post_id))
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
  groups := self.daemon.database.GetAllNewsgroups()
  if groups != nil {
    for _, group := range groups {
      self.regenerateBoard(group)
    }
  }
}

func (self httpFrontend) regenerateBoard(newsgroup string) {
  if self.daemon.database.GroupHasPosts(newsgroup) {
    self.daemon.database.GetGroupThreads(newsgroup, self.regenThreadChan)
  }
}

func (self httpFrontend) regenerateThread(rootMessageID string) {
  var replies []string
  if self.daemon.database.ThreadHasReplies(rootMessageID) {
    replies = self.daemon.database.GetThreadReplies(rootMessageID, 0)
  }
  msg := self.daemon.store.GetMessage(rootMessageID)
  if msg == nil {
    log.Printf("failed to fetch root post %s, regen cancelled", rootMessageID)
    return
  }

  post := PostModelFromMessage(msg)
  posts := []PostModel{post}
  
  for _, msgid := range replies {
    msg = self.daemon.store.GetMessage(msgid)
    if msg == nil {
        log.Println("could not get message", msgid)
      continue
    }
    post = PostModelFromMessage(msg)
    posts = append(posts, post)
  }
  thread := NewThreadModel(posts)
  // render the thread
  fname := self.getFilenameForThread(rootMessageID)
  wr, err := OpenFileWriter(fname)
  if err != nil {
    log.Println(err)
    return
  }
  err = thread.RenderTo(wr)
  wr.Close()
  if err != nil {
    log.Printf("failed to render %s", err)
  }
  log.Printf("regenerated file %s", fname)
}

func (self httpFrontend) poll() {
  chnl := self.PostsChan()
 
  for {
    select {
    case nntp := <- chnl:
      if len(nntp.Reference) > 0 {
        self.regenThreadChan <- nntp.Reference
      } else {
        self.regenThreadChan <- nntp.MessageID
      }
      // todo: regen board pages
      break
    }
  }
}
// select loop for channels
func (self httpFrontend) pollregen() {
  for {
    select {
    case msgid := <- self.regenThreadChan:
      self.regenerateThread(msgid)
      break
    }
  }
}

func (self httpFrontend) renderPostForm(wr io.Writer, board, ref string) {
  // get form template path
  fname := filepath.Join(defaultTemplateDir(), "postform.mustache")
  // template param
  param := make(map[string]string)
  param["post_url"] = self.prefix + board
  param["reference"] = ref
  param["newsgroup"] = "overchan." + board
  param["button"] = "post it nigguh"
  // TODO: implement captcha
  param["captcha_id"] = ""
  param["captcha_img"] = ""
  // render and write
  io.WriteString(wr, templateRender(fname, param))
}

func (self httpFrontend) handle_postform(wr http.ResponseWriter, r *http.Request, board string) {
  // make the message
  nntp := new(NNTPMessage)
  // generate message id
  nntp.MessageID = fmt.Sprintf("<%s%d@%s>", randStr(12), timeNow(), self.name)
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
  if r.Method == "POST" {
    self.handle_postform(wr, r, board)
  } else if r.Method == "GET" {
    // get method
    // generate post form
    if len(board) > 0 {
      self.renderPostForm(wr, board, "")
    } else {
      wr.WriteHeader(404)
      io.WriteString(wr, "No Such board")
    }
  }
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
  front.regenGroupChan = make(chan string, 8)
  return front
}
