//
// frontend.go
// srnd static html frontend
//
//
package srnd

import (
  "io"
  "log"
  "net/http"
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
  
  postchan chan *NNTPMessage
  bindaddr string
  name string
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

func (self httpFrontend) Mainloop() {
  // start webserver here
  log.Printf("frontend %s binding to %s", self.name, self.bindaddr)
  err := http.ListenAndServe(self.bindaddr, self)
  if err != nil {
    log.Fatalf("failed to bind frontend %s %s", self.name, err)
  }
}


// create a new http based frontend
func NewHTTPFrontend(bindaddr, name string) Frontend {
  var front httpFrontend
  front.bindaddr = bindaddr
  front.name = name
  front.postchan = make(chan *NNTPMessage, 16)
  return front
}
