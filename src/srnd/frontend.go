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
)

// frontend interface for any type of frontend
type Frontend interface {

  // channel that is for the nntpd to poll for new posts from this frontend
  NewPostsChan() chan *NNTPMessage

  // bind any network sockets
  Bind()
  
  // run mainloop
  Mainloop()
}

// muxed frontend for holding many frontends
type multiFrontend struct {
  Frontend

  muxedchan chan *NNTPMessage
  frontends []Frontend
}


func (self multiFrontend) Mainloop() {
  for idx := range(self.frontends) {
    go self.frontends[idx].Mainloop()
    go self.forwardPosts(self.frontends[idx])
  }
}

func (self multiFrontend) Bind() {
  for idx := range (self.frontends) {
    self.frontends[idx].Bind()
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
      self.muxedchan <- nntp
    }
  }
}

func (self multiFrontend) NewPostsChan() chan *NNTPMessage {
  return self.muxedchan
}



func MuxFrontends(fronts ...Frontend) Frontend {
  var front multiFrontend
  front.muxedchan = make(chan *NNTPMessage, 128)
  front.frontends = fronts
  return front
}

type httpFrontend struct {
  Frontend

  daemon *NNTPDaemon
  postchan chan *NNTPMessage
  bindaddr string
  name string

  webroot_dir string
  template_dir string

  prefix string
  regenThreadChan chan string
  regenGroupChan chan string
}

func (self httpFrontend) getFilenameForThread(root_post_id string) string {
  fname := fmt.Sprintf("thread-%s.html", ShortHashMessageID(root_post_id))
  return filepath.Join(self.webroot_dir, fname)
}

func (self httpFrontend) Bind() {
  
}

func (self httpFrontend) NewPostsChan() chan *NNTPMessage {
  return self.postchan
}

func (self httpFrontend) loghttp(req *http.Request, code int) {
  log.Printf("%s -- %s %s -- %d", self.name, req.Method, req.URL.Path, code)
}

func (self httpFrontend) ServeHTTP(wr http.ResponseWriter, req *http.Request) {
  io.WriteString(wr, "works")
  self.loghttp(req, 200)
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
  } else {
    replies = nil
  }
  msg := self.daemon.store.GetMessage(rootMessageID)
  if msg == nil {
    log.Printf("failed to fetch root post %s, regen cancelled", rootMessageID)
    return
  }

  post := PostModelFromMessage(msg)
  
  thread := NewThreadModel(post)
  if replies != nil {
    for _, msgid := range replies {
      msg = self.daemon.store.GetMessage(msgid)
      post = PostModelFromMessage(msg)
      thread.AddPost(post)
    }
  }
  // render the thread
  fname := self.getFilenameForThread(rootMessageID)
  wr, err := OpenFileWriter(fname)
  if err != nil {
    log.Printf("failed to open %s, %s", fname, err)
    return
  }
  err = thread.RenderTo(wr)
  wr.Close()
  if err != nil {
    log.Printf("failed to render %s, %s", fname, err)
  }
}

// select loop for regenerating pages
func (self httpFrontend) pollregen() {
  for {
    select {
    case msgid := <- self.regenThreadChan:
      go self.regenerateThread(msgid)

    }
  }
}

func (self httpFrontend) Mainloop() {
  EnsureDir(self.webroot_dir)
  if ! CheckFile(self.template_dir) {
    log.Fatalf("no such template folder %s", self.template_dir)
  }
  // poll for regenerate thread
  go self.pollregen()
  // start webserver here
  log.Printf("frontend %s binding to %s", self.name, self.bindaddr)
  err := http.ListenAndServe(self.bindaddr, self)
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
  front.regenThreadChan = make(chan string, 16)
  front.regenGroupChan = make(chan string, 8)
  return front
}
