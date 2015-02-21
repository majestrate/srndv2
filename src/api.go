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
  client net.Conn
}

type API_File struct {
  mime string
  extension string
  name string
  data string
}

// api message for articles
type API_Article struct {
  please string
  id string
  newsgroup string
  op bool
  thread string
  name string
  sage bool
  key string
  subject string
  comment string
  files []API_File
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
    self.handleClient(conn)
    self.client = nil
    log.Println("api connection done")
  }
}

// write a json object to the client with the delimiter
func (self *SRNdAPI) write_Client(obj interface{}) error {
  
  var buff bytes.Buffer
  // marshal json to bytes
  raw, err := json.Marshal(obj)
  if err != nil {
    return err
  }
  
  // write bytes to buffer with delimeter
  buff.Write(raw)
  buff.WriteString("\n.\n")
  
  // write out to client
  _, err = self.client.Write(buff.Bytes())
  return err
}

func (self *SRNdAPI) handle_Socket(socket string) {
  
  conn, err := net.Dial("unix", socket)
  if err != nil {
    log.Println("api error", err)
    return
  }
  log.Println("api client socket set to", socket)
  self.client = conn
}

func (self *SRNdAPI) sendMessage(message *NNTPMessage) error {
  msg := message.APIMessage()
  return self.write_Client(msg)
}

func (self *SRNdAPI) handle_SyncNewsgroup(newsgroup string) error {
  var err error
  store := self.daemon.store
  store.iterAllForNewsgroup(newsgroup, func (article_id string) error {
    msg := store.GetMessage(article_id, true)
    if msg == nil {
      return nil
    }
    err = self.sendMessage(msg)
    return err
  })
  return err
}

// handle an incoming json object
func (self *SRNdAPI) handleMessage(m API_InMessage) {
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
    var newsgroups []string
    newsgroups = val.([]string)
    if len(newsgroups) > 0 {
      for idx := range newsgroups {
        group := newsgroups[idx]
        err := self.handle_SyncNewsgroup(group)
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
func (self *SRNdAPI) handleClient(incoming io.ReadWriteCloser) {
  
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
