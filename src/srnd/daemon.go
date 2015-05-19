//
// daemon.go
//
package srnd
import (
  "log"
  "net"
  "strconv"
  "strings"
  "net/textproto"
  "time"
)

type NNTPDaemon struct {
  instance_name string
  bind_addr string
  conf *SRNdConfig
  store *ArticleStore
  database Database
  listener net.Listener
  debug bool
  sync_on_start bool
  running bool
  // http frontend
  frontend Frontend

  // nntp feeds map, feed, isoutbound
  feeds map[NNTPConnection]bool
  infeed chan *NNTPMessage
  // channel to load messages to infeed given their message id
  infeed_load chan string
  // channel for broadcasting a message to all feeds given their message id
  send_all_feeds chan string
}

func (self *NNTPDaemon) End() {
  self.listener.Close()
}


// register a new connection
// can be either inbound or outbound
func (self *NNTPDaemon) newConnection(conn net.Conn, inbound bool, policy *FeedPolicy) NNTPConnection {
  feed := NNTPConnection{conn, textproto.NewConn(conn), inbound, self.debug, new(ConnectionInfo), policy, make(chan *NNTPMessage, 64),  make(chan string, 512)}
  self.feeds[feed] = ! inbound
  return feed
}

func (self *NNTPDaemon) persistFeed(conf FeedConfig) {
  for {
    if self.running {
      
      var conn net.Conn
      var err error
      proxy_type := strings.ToLower(conf.proxy_type)
      
      if proxy_type ==  "" || proxy_type == "none" {
        // connect out without proxy 
        log.Println("dial out to ", conf.addr)
        conn, err = net.Dial("tcp", conf.addr)
        if err != nil {
          log.Println("cannot connect to outfeed", conf.addr, err)
					time.Sleep(5)
          continue
        }
      } else if proxy_type == "socks4a" {
        // connect via socks4a
        log.Println("dial out via proxy", conf.proxy_addr)
        conn, err = net.Dial("tcp", conf.proxy_addr)
        if err != nil {
          log.Println("cannot connect to proxy", conf.proxy_addr)
					time.Sleep(5)
          continue
        }
        // generate request
        idx := strings.LastIndex(conf.addr, ":")
        if idx == -1 {
          log.Fatal("invalid outfeed address")
        }
        var port uint64
        addr := conf.addr[:idx]
        port, err = strconv.ParseUint(conf.addr[idx+1:], 10, 16)
        if port >= 25536 {
          log.Fatal("bad proxy port" , port)
        }
        var proxy_port uint16
        proxy_port = uint16(port)
        proxy_ident := "srndv2"
        req_len := len(addr) + 1 + len(proxy_ident) + 1 + 8

        req := make([]byte, req_len)
        // pack request
        req[0] = '\x04'
        req[1] = '\x01'
        req[2] = byte(proxy_port & 0xff00 >> 8)
        req[3] = byte(proxy_port & 0x00ff)
        req[7] = '\x01'
        idx = 8
        
        proxy_ident_b := []byte(proxy_ident)
        addr_b := []byte(addr)
        
        var bi int
        for bi = range proxy_ident_b {
          req[idx] = proxy_ident_b[bi]
          idx += 1
        }
        idx += 1
        for bi = range addr_b {
          req[idx] = addr_b[bi]
          idx += 1
        }
  
        // send request
        conn.Write(req)
        resp := make([]byte, 8)
        
        // receive response
        conn.Read(resp)
        if resp[1] == '\x5a' {
          // success
          log.Println("connected to", conf.addr)
        } else {
          log.Println("failed to connect to", conf.addr)
					time.Sleep(5)
          continue
        }
      }
      policy := &conf.policy
      nntp := self.newConnection(conn, false, policy)
      nntp.HandleOutbound(self)
      log.Println("remove outfeed")
      delete(self.feeds, nntp)
    }
  }
  time.Sleep(1 * time.Second)
}

// sync every article to all feeds
func (self *NNTPDaemon) syncAll() {
  log.Println("sync all feeds")
  chnl := make(chan string, 128)
  go self.store.IterateAllArticles(chnl)
  for {
    select {
    case msgid := <- chnl:
      for feed, use := range self.feeds {
        if use {
          feed.sync <- msgid
        }
      }
    }
  }
}

// run daemon
func (self *NNTPDaemon) Run() {	
  err := self.Bind()
  if err != nil {
    log.Println("failed to bind:", err)
    return
  }
  defer self.listener.Close()

  // we are now running
  self.running = true
  
  // persist outfeeds
  for idx := range self.conf.feeds {
    go self.persistFeed(self.conf.feeds[idx])
  }

  // start accepting incoming connections
  go self.acceptloop()
  
  if self.sync_on_start {
    go self.syncAll()
  }
  // if we have no frontend this does nothing
  if self.frontend != nil {
    go self.pollfrontend()
  }
  self.pollfeeds()

}


func (self *NNTPDaemon) pollfrontend() {
  for {
    
    select {
    case nntp := <- self.frontend.NewPostsChan():
      // new post from frontend
      // ammend path
      nntp.Path = self.instance_name + "!" + nntp.Path
      // tell infeed that we got one
      self.infeed <- nntp
      break
    }
  }
}

func (self *NNTPDaemon) pollfeeds() {
  for {
    select {
    case msgid := <- self.send_all_feeds:
      // send all feeds 
      nntp := self.store.GetMessage(msgid)
      for feed , use := range self.feeds {
        if use && feed.policy.AllowsNewsgroup(nntp.Newsgroup) {
          feed.send <- nntp
        }
      }
      break
    case nntp := <- self.infeed:
      // register article
      self.database.RegisterArticle(nntp)
      // store article
      self.store.StorePost(nntp)
      // queue to all outfeeds
      self.send_all_feeds <- nntp.MessageID
      
      break
    case msgid := <- self.infeed_load:
      // load temp message
      // this deletes the temp file
      nntp := self.store.ReadTempMessage(msgid)
      // rewrite path header
      nntp.Path = self.instance_name +"!" + nntp.Path
      // offer infeed
      self.infeed <- nntp
      break
    }
  }
}

func (self *NNTPDaemon) acceptloop() {	
  for {
    // accept
    conn, err := self.listener.Accept()
    if err != nil {
      log.Fatal(err)
    }
    // make a new inbound nntp connection handler 
    nntp := self.newConnection(conn, true, nil)
    go self.RunInbound(nntp)
  }
}

func (self *NNTPDaemon) RunInbound(nntp NNTPConnection) {
  nntp.HandleInbound(self)
  delete(self.feeds, nntp)
}

// bind to address
func (self *NNTPDaemon) Bind() error {
  listener , err := net.Listen("tcp", self.bind_addr)
  if err != nil {
    log.Println("failed to bind to", self.bind_addr, err)
    return err
  }
  self.listener = listener
  log.Println("SRNd NNTPD bound at", listener.Addr())
  return nil
}

// load configuration
// bind to interface
func (self *NNTPDaemon) Init() bool {
  CheckConfig()
  log.Println("load config")
  self.conf = ReadConf()
  if self.conf == nil {
    log.Println("cannot load config")
    return false
  }
  self.infeed = make(chan *NNTPMessage, 16)
  self.infeed_load = make(chan string, 16)
  self.send_all_feeds = make(chan string, 16)
  self.feeds = make(map[NNTPConnection]bool)
  
  
  db_host := self.conf.database["host"]
  db_port := self.conf.database["port"]
  db_user := self.conf.database["user"]
  db_passwd := self.conf.database["password"]

  self.database = NewDatabase(self.conf.database["type"])
  
  err := self.database.Init(db_host, db_port, db_user, db_passwd)
  if err != nil {
    log.Println("failed to initialize database", err)
    return false
  }
  self.database.CreateTables()
  
  self.store = new(ArticleStore)
  self.store.directory = self.conf.store["store_dir"]
  self.store.temp = self.conf.store["incoming_dir"]
  self.store.database = self.database
  self.store.Init()
  
  self.sync_on_start = self.conf.daemon["sync_on_start"] == "1"
  if self.sync_on_start {
    log.Println("sync on start") 
  }
  self.bind_addr = self.conf.daemon["bind"]
  self.debug = self.conf.daemon["log"] == "debug"
  self.instance_name = self.conf.daemon["instance_name"]
  if self.debug {
    log.Println("debug mode activated")
  }

  
  // do we enable the frontend?
  if self.conf.frontend["enable"] == "1" {
    log.Printf("frontend %s enabled", self.conf.frontend["name"]) 
    self.frontend = NewHTTPFrontend(self.conf.frontend["bind"], self.conf.frontend["name"]) 
    go self.frontend.Mainloop()
  }
  
  return true
}
