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
  client *bufio.Writer
}

type API_File struct {
  mime string
  extension string
  filename string
  data string
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
    var socket string
    switch val.(type) {
      case string:
        socket = val.(string)
      default:
        log.Println("wtf", val)
    }
    conn, err := net.Dial("unix", socket)
    if err != nil {
      log.Println("api error", err)
      return
    }
    log.Println("api client socket set to", socket)
    self.client = bufio.NewWriter(conn)
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
