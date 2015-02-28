//
// api.go
//
package main


import (
  "bufio"
  "bytes"
  "encoding/json"
  "io"
  "log"
  "net"
)

type SRNdAPI struct {
  listener net.Listener
  config *APIConfig
  daemon *NNTPDaemon
}

type SRNdAPI_Handler struct {
  daemon *NNTPDaemon
  client net.Conn
}

// api message for incoming to daemon
type API_InMessage map[string]interface{} 


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
}

func (self *SRNdAPI) Mainloop() {
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
    go handler.handleClient(conn)
  }
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
    //log.Println("loaded", article_id)
    err = self.sendMessage(msg)
    return err != nil
  })
  return err
}

func (self *SRNdAPI_Handler) handle_SyncAllNewsgroups() error {
  var err error 
  store := self.daemon.store
  store.IterateAllArticles(func (article_id string) bool {
    msg := store.GetMessage(article_id, true)
    if msg == nil {
      log.Println("could not load message", article_id)
      return false
    }
    //log.Println("loaded", article_id)
    err = self.sendMessage(msg)
    if err != nil {
      log.Println("error sending message", err)
      return true
    }
    return false
  })
  return err
}

// handle an incoming json object
func (self *SRNdAPI_Handler) handleMessage(m API_InMessage) {
  please, ok := m["please"]
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
        self.handleMessage(message)
      } else {
        log.Println("api got bad json:",err)
        break
      }
    } else {
      buff.Write(line)
    }
  }
  incoming.Close()
  log.Println("api connection closed")
}
