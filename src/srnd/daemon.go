//
// daemon.go
//
package srnd
import (
  "log"
  "net"
  "fmt"
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
  mod Moderation
  expire Expiration
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
  feed := NNTPConnection{conn, textproto.NewConn(conn), inbound, self.debug, new(ConnectionInfo), policy, make(chan *NNTPMessage, 64),  make(chan string, 512), false}
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
					time.Sleep(time.Second)
          continue
        }
      } else if proxy_type == "socks4a" {
        // connect via socks4a
        log.Println("dial out via proxy", conf.proxy_addr)
        conn, err = net.Dial("tcp", conf.proxy_addr)
        if err != nil {
          log.Println("cannot connect to proxy", conf.proxy_addr)
					time.Sleep(time.Second)
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
      // start syncing in background
      go func() {
        // get every article
        articles := self.database.GetAllArticles()
        // wait 5 seconds for feed to handshake
        time.Sleep(5 * time.Second)
        log.Println("outfeed begin sync")
        for _, result := range articles {
          msgid := result[0]
          group := result[1]
          if policy.AllowsNewsgroup(group) {
            //XXX: will this crash if interrupted?
            nntp.sync <- msgid
          }
        }
        log.Println("outfeed end sync")
      }()
      nntp.HandleOutbound(self)
      log.Println("remove outfeed")
      delete(self.feeds, nntp)
    }
  }
  time.Sleep(1 * time.Second)
}

// sync every article to all feeds
func (self *NNTPDaemon) syncAll() {
  
}


// run daemon
func (self *NNTPDaemon) Run() {	
  err := self.Bind()
  if err != nil {
    log.Println("failed to bind:", err)
    return
  }
  defer self.listener.Close()
  // run expiration mainloop
  go self.expire.Mainloop()
  // we are now running
  self.running = true
  
  // persist outfeeds
  for idx := range self.conf.feeds {
    go self.persistFeed(self.conf.feeds[idx])
  }

  // start accepting incoming connections
  go self.acceptloop()

  go func () {
    // if we have no initial posts create one
    if self.database.ArticleCount() == 0 {
      nntp := new(NNTPMessage)
      nntp.Newsgroup = "overchan.overchan"
      nntp.MessageID = fmt.Sprintf("<%s%d@%s>", randStr(5), timeNow(), self.instance_name)
      nntp.Name = "system"
      nntp.Email = "srndv2@"+self.instance_name
      nntp.Subject = "New Frontend"
      nntp.Posted = timeNow()
      nntp.Message = "Hi, welcome to nntpchan, this post was inserted on startup because you have no other posts, this messages was auto-generated"
      nntp.ContentType = "text/plain"
      nntp.Path = self.instance_name
      file := self.store.CreateTempFile(nntp.MessageID)
      if file != nil {
        nntp.WriteTo(file, "\r\n")
        file.Close()
        self.infeed <- nntp
      }
    }
  }()
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
  chnl := self.frontend.NewPostsChan()
  for {
    select {
    case nntp := <- chnl:
      // new post from frontend
      log.Println("frontend post", nntp.MessageID)
      self.infeed <- nntp
    }
  }
}

func (self *NNTPDaemon) pollfeeds() {
  chnl := self.frontend.PostsChan()
  for {
    select {
    case msgid := <- self.send_all_feeds:
      // send all feeds
      nntp := self.store.GetMessage(msgid)
      if nntp == nil {
        log.Printf("failed to load %s for federation", msgid)
      } else {
        for feed , use := range self.feeds {
          if use && feed.policy != nil {
            if feed.policy.AllowsNewsgroup(nntp.Newsgroup) {
              feed.sync <- nntp.MessageID
              log.Println("told feed")
            } else {
              log.Println("not syncing", msgid)
            }
          }
        }
      }
    case nntp := <- self.infeed:
      // ammend path
      nntp.Path = self.instance_name + "!" + nntp.Path
      // check for validity
      log.Println("daemon got", nntp.MessageID)
      if nntp.Verify() {
        // register article
        self.database.RegisterArticle(nntp)
        // store article
        // this generates thumbs and stores attachemnts
        self.store.StorePost(nntp)
        // queue to all outfeeds
        self.send_all_feeds <- nntp.MessageID
        // roll over old content
        // TODO: hard coded expiration threshold
        self.expire.ExpireGroup(nntp.Newsgroup, 100)
        // tell frontend
        chnl <- nntp
        // do any moderation events
        nntp.DoModeration(&self.mod)
      } else {
        log.Printf("%s has invalid signature", nntp.MessageID)
      }
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
  self.infeed = make(chan *NNTPMessage, 64)
  self.infeed_load = make(chan string, 64)
  self.send_all_feeds = make(chan string, 64)
  self.feeds = make(map[NNTPConnection]bool)
  

  db_host := self.conf.database["host"]
  db_port := self.conf.database["port"]
  db_user := self.conf.database["user"]
  db_passwd := self.conf.database["password"]

  self.database = NewDatabase(self.conf.database["type"], self.conf.database["schema"], db_host, db_port, db_user, db_passwd)
  self.database.CreateTables()
  
  self.store = new(ArticleStore)
  self.store.directory = self.conf.store["store_dir"]
  self.store.temp = self.conf.store["incoming_dir"]
  self.store.attachments = self.conf.store["attachments_dir"]
  self.store.thumbs = self.conf.store["thumbs_dir"]
  self.store.database = self.database
  self.store.Init()
  
  self.expire = expire{self.database, self.store, make(chan deleteEvent)}
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

  // initialize moderation engine
  self.mod.Init(self)
  
  // do we enable the frontend?
  if self.conf.frontend["enable"] == "1" {
    log.Printf("frontend %s enabled", self.conf.frontend["name"]) 
    self.frontend = NewHTTPFrontend(self, self.conf.frontend) 
    go self.frontend.Mainloop()
  }
  
  return true
}
