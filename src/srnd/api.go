//
// api.go
//
package srnd


import (
  "bufio"
  "bytes"
  "encoding/json"
  "errors"
  "fmt"
  "io"
  "log"
  "net"
)


type SRNdAPI_Handler struct {

  daemon *NNTPDaemon
  client net.Conn
  name string
}

type SRNdAPI struct {
  listener net.Listener
  config *APIConfig
  daemon *NNTPDaemon
  infeed chan *NNTPMessage
  senders []*SRNdAPI_Handler
}


// api message for incoming to daemon
type API_InMessage map[string]interface{} 

func (self API_InMessage) GetString(key string) string {
  if self[key] == nil {
    return ""
  }
  return fmt.Sprintf("%s", self[key])
}

func (self API_InMessage) GetLong(key string) int64 {
  return int64(self[key].(float64))
}

func (self API_InMessage) GetBool(key string) bool {
  return self.GetString(key) == "true"
}

func (self API_InMessage) IsPost() bool {
  return self.GetString("Please") == "post"
}

func (self API_InMessage) GetArray(key string) []interface{} {
  if self[key] == nil {
    return nil
  }
  return self[key].([]interface{})
}


func (self API_InMessage) GetFiles() []NNTPAttachment {
  // get array of files
  files := self.GetArray("Files")
  // count files
  filecount := len(files)
  var postfiles []NNTPAttachment
  // if we have files put them in it
  if filecount > 0 {
    postfiles = make([]NNTPAttachment, filecount)
    for idx := range(postfiles) {
      file := files[idx].(map[string]string)
      // parse file
      postfiles[idx].Mime = file["Mime"]
      postfiles[idx].Extension = file["Extension"]
      postfiles[idx].Name = file["Name"]
      postfiles[idx].Data = file["Data"]
    }
  }
  return postfiles
}

// convert this api message into a post message because strong typing
func (self API_InMessage) ToNNTP() NNTPMessage {
  var post NNTPMessage
  post.MessageID = self.GetString("MessageID")
  post.Newsgroup = self.GetString("Newsgroup")
  post.OP = self.GetBool("OP")
  post.Sage = self.GetBool("Sage")
  post.Reference = self.GetString("Reference")
  post.Posted = self.GetLong("Posted")
  post.Key = self.GetString("Key")
  post.Subject = self.GetString("Subject")
  post.Message = self.GetString("Comment")
  post.Name = self.GetString("Name")
  post.Email = self.GetString("Email")
  post.Attachments = self.GetFiles()
  if len(post.Attachments) == 0 {
    post.ContentType = "text/plain; encoding=UTF-8"
  }
  return post
}

func (self *SRNdAPI) Init(d *NNTPDaemon) {
  var err error
  log.Println("initialize api...")
  self.config = &d.conf.api
  addr := self.config.srndAddr
  if CheckFile(addr) {
    log.Println("deleting old socket ", addr)
    DelFile(addr)
  }
  self.listener, err = net.Listen("unix", addr)
  if err != nil {
    log.Fatal("cannot make socket file ", addr)
    return
  }
  self.daemon = d
  self.infeed = make(chan *NNTPMessage, 16)
  // 8 api handlers initially
  self.senders = make([]*SRNdAPI_Handler, 8)
}


// tell all senders about a new post from backend
func (self *SRNdAPI) informSenders(msg *NNTPMessage) {
  log.Println("api got message", msg.MessageID)
  for idx := range(self.senders) {
    sender := self.senders[idx]
    if sender != nil {
      sender.sendMessage(msg)
    }
  }
}

func (self *SRNdAPI) Mainloop() {
  go func () {
    for {
      select {
      case msg := <- self.infeed:
        self.informSenders(msg)
      }
    }
  }()
  for {
    conn, err := self.listener.Accept()
    if err != nil {
      log.Println("failure to accept incoming api connection", err)
      break
    }
    log.Println("new api connection")
    handler := new(SRNdAPI_Handler)
    handler.daemon = self.daemon
    handler.client = conn
    go func() {
      self.RegisterSender(handler)
      handler.handleClient(conn)
      self.DeRegisterSender(handler)
    }()
  }
}

// register as sender for sending nntp messages to
func (self *SRNdAPI) RegisterSender(sender *SRNdAPI_Handler) {
  // find a nil slot
  for idx := range(self.senders) {
    // put it into that empty slot
    if self.senders[idx] == nil {
      self.senders[idx] = sender
      return
    }
  }
  // if we can't find an empty slot make the array bigger
  self.senders = append(self.senders, sender)
}

func (self *SRNdAPI) DeRegisterSender(sender *SRNdAPI_Handler) {
  // find it and set it to nil in the array
  for idx := range(self.senders) {
    if self.senders[idx] == sender {
      self.senders[idx] = nil
      return
    }
  }
  // we didn't find it
  log.Println("did not deregister sender, not registered?")
}

// write a json object to the client with the delimiter
func (self *SRNdAPI_Handler) write_Client(obj interface{}) error {
  
  var buff bytes.Buffer
  // marshal json to bytes
  raw, err := json.Marshal(obj)
  if err != nil {
    log.Println("error marshaling json", err)
    return err
  }
  
  // write bytes to buffer with delimeter
  buff.Write(raw)
  buff.WriteString("\n.\n")
  // write out to client
  _, err = self.client.Write(buff.Bytes())
  return err
}

func (self *SRNdAPI_Handler) handle_Socket(socket string) {
  self.name = socket
  conn, err := net.Dial("unix", socket)
  if err != nil {
    log.Println("api error", err)
    return
  }
  log.Println("api client socket set to", socket)
  self.client = conn
}

func (self *SRNdAPI_Handler) sendMessage(message *NNTPMessage) error {
  return self.write_Client(message)
}

func (self *SRNdAPI_Handler) handle_SyncNewsgroup(newsgroup string) error {
  var err error
  store := self.daemon.store
  store.IterateAllForNewsgroup(newsgroup, func (article_id string) bool {
    msg := store.GetMessage(article_id, true)
    if msg == nil {
      log.Println("could not load message", article_id)
      return false
    }
    err = self.sendMessage(msg)
    return err != nil
  })
  return err
}

func (self *SRNdAPI_Handler) handle_Post(msg API_InMessage) error {
  store := self.daemon.store
  feed := self.daemon.infeed
  post := msg.ToNNTP()
  // set path header as from us
  post.Path = fmt.Sprintf("%s!%s", self.daemon.instance_name, self.name)

  if store.HasArticle(post.MessageID) {
    return errors.New("already have article "+post.MessageID)
  }

  err := store.StorePost(&post)
  if err == nil {
    log.Println("Got New Post From",self.name)
    // inform daemon in feed
    feed <- post.MessageID
  } else {
    log.Println("error while posting", err)
  }
  return err
}

func (self *SRNdAPI_Handler) handle_SyncAllNewsgroups() error {
  var err error 
  store := self.daemon.store
  store.IterateAllArticles(func (article_id string) bool {
    // load entire body
    msg := store.GetMessage(article_id, true)
    if msg == nil {
      log.Println("could not load message", article_id)
      return false
    }
    err = self.sendMessage(msg)
    if err != nil {
      log.Println("error sending message", err)
      return true
    }
    return false
  })
  return err
}

// handle an incoming json object control message
func (self *SRNdAPI_Handler) handle_Control(m API_InMessage) {
  please, ok := m["Please"]
  var val interface{}
  if ! ok {
    log.Println("invalid api command")
    return
  }
  if please == "socket" {
    val = m["socket"]
    self.handle_Socket(val.(string))
  } else if please == "sync" {
    val = m["newsgroups"]
    newsgroups := val.([]interface{})
    if len(newsgroups) > 0 {
      for idx := range newsgroups {
        group := newsgroups[idx]
        err := self.handle_SyncNewsgroup(group.(string))
        if err != nil {
          log.Println("error syncing", group, err)
        }
      }
    } else {
      self.handle_SyncAllNewsgroups()
    }
  }
}

// handle a client connection
func (self *SRNdAPI_Handler) handleClient(incoming io.ReadWriteCloser) {
  reader := bufio.NewReader(incoming)
  var buff bytes.Buffer
  var message API_InMessage
  for {
    line, err := reader.ReadBytes('\n')
    if err != nil {
      break
    }
    // eof
    if len(line) == 0 {
      break
    }
    if line[0] == '.' {
      log.Println("api got command")
      // marshal buffer to json
      raw := buff.Bytes()
      buff.Reset()
      err = json.Unmarshal(raw, &message)
      // handle json 
      // drop connection if json is invalid
      if err == nil {
        if message.IsPost() {
          // we got a post from the frontend
          err = self.handle_Post(message)
          if err != nil {
            log.Println("failed to handle post: ", err)
          }
        } else {
          // we got a control message from the frontend
          self.handle_Control(message)
        }
      } else {
        log.Println("api got bad json: ",err)
        break
      }
    } else {
      buff.Write(line)
    }
  }
  incoming.Close()
  log.Println("api connection closed")
}
