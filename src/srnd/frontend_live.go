//
// frontend_live.go -- livechan style ui via websockets
//

package srnd

import (
  "container/list"
  "encoding/json"
  "fmt"
  "github.com/dchest/captcha"
  "github.com/gorilla/mux"
  "github.com/gorilla/sessions"
  "github.com/gorilla/websocket"
  "log"
  "net"
  "net/http"
  "time"
)

type liveFrontend struct {
  obPostChan chan NNTPMessage
  ibPostChan chan NNTPMessage

  // channel for unsubscribing/subscribing to events
  subscribeSocketChan chan chan interface{}
  unsubscribeSocketChan chan chan interface{}
  
  config *liveConfig
  daemon *NNTPDaemon
  
  session *sessions.CookieStore
  wsocket *websocket.Upgrader
  httpmux *mux.Router
}


func (self liveFrontend) NewPostsChan() chan NNTPMessage {
  return self.obPostChan
}

func (self liveFrontend) PostsChan() chan NNTPMessage {
  return self.ibPostChan
}

// only allow nntpchan.live for now
func (self liveFrontend) AllowNewsgroup(group string) bool {
  return group == "nntpchan.live"
  // return strings.HasPrefix(group, "nntpchan.")
}

// this does nothing as everything is streamed
func (self liveFrontend) Regen(msg ArticleEntry) {
}

// make a notice to then serialize as json to be sent down a websocket
func (self liveFrontend) makeNotice(msg string) interface{} {
  notice := make(map[string]string)
  notice["cmd"] = "notice"
  notice["data"] = msg
  return notice
}

func (self *liveFrontend) handle_websocket(wr http.ResponseWriter, r *http.Request) {
  addr, _, _ := net.SplitHostPort(r.RemoteAddr)
  // check for ip ban
  banned, err := self.daemon.database.CheckIPBanned(addr)
  if err == nil {
    if banned {
      // user is banned
      http.Error(wr, "banned", 403)
      return
    }
  } else {
    // error checking ban
    http.Error(wr, err.Error(), 503)
    return
  }
  // begin websocket upgrade
  wsconn, err := self.wsocket.Upgrade(wr, r, nil)
  if err == nil {
    // handshake okay we websocket now
    // read messages until we get a valid captcha solution
    for {
      req := make(map[string]interface{})
      err = wsconn.ReadJSON(&req)
      if self.check_captcha(req) {
        // valid captcha we gud
        log.Println("captcha gud from", wsconn.RemoteAddr())
        break
      } else {
        // invalid captcah try again
        log.Println("invalid captcha from", wsconn.RemoteAddr())
        continue
      }
    }
    // if we reach here that means we have completed captcha
    // tell the ui we are ready
    ready := map[string]string { "cmd" : "ready" }
    err = wsconn.WriteJSON(&ready)
    // send greeting
    greeting := self.makeNotice("Welcome to nntpchan. Posts will come in live as they are recieved.")
    err = wsconn.WriteJSON(&greeting)
    // TODO: send some posts too?

    // handle read events
    go self.handle_recv_websocket(wsconn)
    
    // subscribe to messages
    chnl := make(chan interface{})
    self.subscribeSocketChan <- chnl
    for err == nil {
      msg, ok := <- chnl
      if ok {
        // we got something to send down
        // write it
        err = wsconn.WriteJSON(&msg)
      } else {
        // channel closed
        break
      }
    }
    // we done
    self.unsubscribeSocketChan <- chnl
  }
  log.Println("error in websocket handler", err)
}


// handle reading of websocket messages
func (self *liveFrontend) handle_recv_websocket(wsconn *websocket.Conn) {
  var err error
  var encaddr string
  // create encrypted address
  addr, _, _ := net.SplitHostPort(wsconn.RemoteAddr().String())
  encaddr, err = self.daemon.database.GetEncAddress(addr)
  // set instance name
  instance := self.daemon.instance_name + ".live"
  for err == nil {
    req := make(map[string]interface{})
    err = wsconn.ReadJSON(&req)
    if err == nil {
      // handle post
      var message_str, name_str, subject_str, group_str string
      // extract parameters
      group, ok := req["group"]
      if ok {
        switch group.(type) {
        default:
          continue
        case string:
          group_str = group.(string)
        }
      } else {
        continue
      }
      message, ok := req["msg"]
      if ok {
        switch message.(type) {
        default:
          continue
        case string:
          message_str = message.(string)
        }
      } else {
        continue
      }
      subject, ok := req["subject"]
      if ok {
        switch subject.(type) {
        default:
          continue
        case string:
          subject_str = subject.(string)
        }
      } else {
        continue
      }
      name, ok := req["name"]
      if ok {
        switch name.(type) {
        default:
          continue
        case string:
          name_str = name.(string)
        }
      } else {
        continue
      }

      att_json, attachment := req["file"]
      
      // do we allow this post?
      if self.AllowNewsgroup(group_str) {
        // yeh
        if name_str == "" {
          name_str = "Anonymous"
        }
        if subject_str == "" {
          subject_str = "None"
        }
        if message_str == "" {
          continue
        }
        var nntp nntpArticle
        nntp.headers = make(ArticleHeaders)
        nntp.message = createPlaintextAttachment(message_str)
        nntp.headers.Set("From", nntpSanitize(fmt.Sprintf("%s <anon@%s", name_str, instance)))
        nntp.headers.Set("Message-ID", genMessageID(nntpSanitize(instance)))
        nntp.headers.Set("Date", timeNowStr())
        nntp.headers.Set("Subject", nntpSanitize(subject_str))
        nntp.headers.Set("X-Encrypted-IP", encaddr)
        nntp.headers.Set("Newsgroups", group_str)
        nntp.headers.Set("Path", nntpSanitize(instance))
        if isSage(subject_str) {
          nntp.headers.Set("X-Sage", "1")
        }
        if attachment {
          att := createAttachmentFromJSON(att_json)
          if att == nil {
            log.Println("failed to parse attachment json")
            continue
          } else {
            nntp = nntp.Attach(att).(nntpArticle)
          }
        }
        nntp.Pack()
        self.obPostChan <- nntp
      } else {
        // nah
        log.Println("liveui got post for disallowed group", group)
        continue
      }
      
    }
  }
  wsconn.Close()
}

// check that this map contains a correctly solved captcha
// return false if fields are missing or the captcha is incorrect
// return true if everything is hunkydory
func (self liveFrontend) check_captcha(req map[string]interface{}) bool {
  id, ok := req["captcha_id"]
  if ok {
    solution, ok := req["captcha_solution"]
    return ok && captcha.VerifyString(id.(string), solution.(string))
  }
  return false
}

// create a new captcha, return as json object
func (self liveFrontend) handle_new_captcha(wr http.ResponseWriter, r *http.Request) {
  captcha_id := captcha.New()
  resp := make(map[string]string)
  // the captcha id
  resp["id"] = captcha_id
  // url of the image
  resp["url"] = fmt.Sprintf("%scaptcha/%s.png", self.config.prefix, captcha_id)
  enc := json.NewEncoder(wr)
  enc.Encode(resp)
}

func (self liveFrontend) marshalMessage(nntp NNTPMessage) interface{} {
  return nil
}

func (self liveFrontend) Mainloop() {
  // create upgrader for websockets
  self.wsocket = &websocket.Upgrader{
    HandshakeTimeout: time.Second * 4,
    ReadBufferSize: self.config.wsRead,
    WriteBufferSize: self.config.wsWrite,
    // TODO: implement origin checks
    CheckOrigin: func(r *http.Request) bool { return true; },
  }
  
  // create mux
  self.httpmux = mux.NewRouter()
  // set routes
  self.httpmux.Path("/ws").HandlerFunc(self.handle_websocket).Methods("POST")
  self.httpmux.Path("/captcha.json").HandlerFunc(self.handle_new_captcha).Methods("GET")
  self.httpmux.Path("/captcha/{f}").Handler(captcha.Server(300, 200)).Methods("GET")
  self.httpmux.Path("/").Handler(http.FileServer(http.Dir(self.config.docroot_dir)))
  
  // run mod engine
  go RunModEngine(self.daemon.mod, nil)
  // bind http server
  go func() {
    log.Println("bind liveui to", self.config.bindAddr)
    err := http.ListenAndServe(self.config.bindAddr, self.httpmux)
    if err != nil {
      log.Fatal("Failed to bind liveui to", self.config.bindAddr, err)
    }
  }()
  
  // all the active channels
  activeChnls := list.New()
  // run channels
  for {
    select {
    case nntp, ok := <- self.ibPostChan:
      // we got an inbound post
      if ok {
        // turn this message into stuff to serialize as json
        msg := self.marshalMessage(nntp)
        // send it to everyone
        for e := activeChnls.Front() ; e != nil ; e = e.Next() {
          chnl := e.Value.(chan interface{})
          chnl <- msg
        }
      }
    case chnl, ok := <- self.subscribeSocketChan:
      if ok {
        // add to active channels
        activeChnls.PushBack(chnl)
      }
    case chnl, ok := <- self.unsubscribeSocketChan:
      if ok {
        // remove from active channels
        for e := activeChnls.Front() ; e != nil ; e = e.Next() {
          if e.Value.(chan interface{}) == chnl {
            // we found it, remove it
            activeChnls.Remove(e)
            // close the channel
            close(chnl)
          }
        }
      }
    }
  }
}

func NewLiveFrontend(daemon *NNTPDaemon, config *liveConfig) Frontend {
  return liveFrontend{
    daemon: daemon,
    config: config,
    ibPostChan: make(chan NNTPMessage, 64),
    obPostChan: make(chan NNTPMessage, 16),
    subscribeSocketChan: make(chan chan interface{}),
    unsubscribeSocketChan: make(chan chan interface{}),
  }
}
