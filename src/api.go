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
  client *io.ReadWriteCloser
}

type API_File struct {
  mime string
  extension string
  filename string
  data string
}

type API_Article struct {
  id string
  newsgroup string
  op bool
  thread string
  frontend string
  sage bool
  subject string
  comment string
  files []API_File
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
}

func (self *SRNdAPI) Mainloop() {
  for {
    conn, err := self.listener.Accept()
    if err != nil {
      log.Println("failure to accept incoming api connection", err)
      break
    }
    go self.handleClient(conn)
  }
}

// handle an incoming json object
func (self *SRNdAPI) handleJSON(raw []byte, j map[string]interface{}) {
  please, ok := j["please"]
  if ok {
    log.Println("please ", please)
    
  }
}

// handle a client connection
func (self *SRNdAPI) handleClient(incoming io.ReadWriteCloser) {
  
  reader := bufio.NewReader(incoming)
  
  for {
    var j map[string]interface{}
    var buff bytes.Buffer
    line, err := reader.ReadBytes(10)
    if line[0] == '.' {
      // marshal buffer to json
      raw := buff.Bytes()
      err = json.Unmarshal(raw, j)
      // handle json 
      // drop connection if json is invalid
      if err == nil {
        self.handleJSON(raw, j)
      } else {
        break
      }
    } else {
      buff.Write(line)
    }
  }
  incoming.Close()
}
