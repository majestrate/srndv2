//
// daemon.go
//
package srnd
import (
  "log"
  "net"
  "net/textproto"
  "strconv"
  "strings"
  "os"
  "time"
)

type NNTPDaemon struct {
  instance_name string
  bind_addr string
  conf *SRNdConfig
  store ArticleStore
  database Database
  mod ModEngine
  expire ExpirationCore
  listener net.Listener
  debug bool
  sync_on_start bool
  // anon settings
  allow_anon bool
  allow_anon_attachments bool
  
  running bool
  // http frontend
  frontend Frontend

  // map of addr -> NNTPConnection
  feeds map[string]nntpConnection
  // for registering and deregistering outbound feeds
  register_outfeed chan nntpConnection
  deregister_outfeed chan nntpConnection
  // thumbnail generator for images
  img_thm ThumbnailGenerator
  infeed chan NNTPMessage
  // channel to load messages to infeed given their message id
  infeed_load chan string
  // channel for broadcasting a message to all feeds given their newsgroup, message_id
  send_all_feeds chan ArticleEntry
  // channel for broadcasting an ARTICLE command to all feeds in reader mode
  ask_for_article chan ArticleEntry
}

func (self NNTPDaemon) End() {
  self.listener.Close()
}


func (self NNTPDaemon) persistFeed(conf FeedConfig, mode string) {
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
					time.Sleep(time.Second * 5)
          continue
        }
      } else if proxy_type == "socks4a" {
        // connect via socks4a
        log.Println("dial out via proxy", conf.proxy_addr)
        conn, err = net.Dial("tcp", conf.proxy_addr)
        if err != nil {
          log.Println("cannot connect to proxy", conf.proxy_addr)
					time.Sleep(time.Second * 5)
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
					time.Sleep(time.Second * 5)
          continue
        }
      }
      nntp := createNNTPConnection()
      nntp.policy = conf.policy
      nntp.name = conf.addr
      c := textproto.NewConn(conn)
      stream, reader, err := nntp.outboundHandshake(c)
      if err == nil {
        self.register_outfeed <- nntp
        if self.sync_on_start {
          go func() {
            log.Println(nntp.name, "will do full sync")
            for _, article := range self.database.GetAllArticles() {
              if nntp.policy.AllowsNewsgroup(article.Newsgroup()) {
                nntp.check <- article.MessageID()
              }
            }
            
          }()
        }
        nntp.runConnection(self, false, stream, reader, c)
        self.deregister_outfeed <- nntp
      } else {
        log.Println("error doing outbound hanshake", err)
      }
    }
  }
  time.Sleep(1 * time.Second)
}

// run daemon
func (self NNTPDaemon) Run() {

  self.bind_addr = self.conf.daemon["bind"]

  listener , err := net.Listen("tcp", self.bind_addr)
  if err != nil {
    log.Fatal("failed to bind to", self.bind_addr, err)
  }
  self.listener = listener
  log.Printf("SRNd NNTPD bound at %s", listener.Addr())

  self.register_outfeed = make(chan nntpConnection)
  self.deregister_outfeed = make(chan nntpConnection)
  self.infeed = make(chan NNTPMessage, 8)
  self.infeed_load = make(chan string)
  self.send_all_feeds = make(chan ArticleEntry, 64)
  self.feeds = make(map[string]nntpConnection)
  self.ask_for_article = make(chan ArticleEntry, 64)

  self.expire = createExpirationCore(self.database, self.store)
  self.sync_on_start = self.conf.daemon["sync_on_start"] == "1"
  self.debug = self.conf.daemon["log"] == "debug"
  self.instance_name = self.conf.daemon["instance_name"]
  self.allow_anon = self.conf.daemon["allow_anon"] == "1"
  self.allow_anon_attachments = self.conf.daemon["allow_anon_attachments"] == "1"
  
  if self.debug {
    log.Println("debug mode activated")
  }
  
  // do we enable the frontend?
  if self.conf.frontend["enable"] == "1" {
    log.Printf("frontend %s enabled", self.conf.frontend["name"]) 
    http_frontend := NewHTTPFrontend(&self, self.conf.frontend, self.conf.worker["url"])
    nntp_frontend := NewNNTPFrontend(&self, self.conf.frontend["nntp"])
    self.frontend = MuxFrontends(http_frontend, nntp_frontend)
    go self.frontend.Mainloop()
  }

  // set up admin user if it's specified in the config
  pubkey , ok := self.conf.frontend["admin_key"]
  if ok {
    // TODO: check for valid format
    log.Println("add admin key", pubkey)
    err = self.database.MarkModPubkeyGlobal(pubkey)
    if err != nil {
      log.Printf("failed to add admin mod key, %s", err)
    }
  }

  
  defer self.listener.Close()
  // run expiration mainloop
  go self.expire.Mainloop()
  // we are now running
  self.running = true
  
  // persist outfeeds
  for idx := range self.conf.feeds {
    go self.persistFeed(self.conf.feeds[idx], "stream")
  }

  // start accepting incoming connections
  go self.acceptloop()

  go func () {
    // if we have no initial posts create one
    if self.database.ArticleCount() == 0 {
      nntp := newPlaintextArticle("welcome to nntpchan, this post was inserted on startup automatically", "system@"+self.instance_name, "Welcome to NNTPChan", "system", self.instance_name, "overchan.test")
      nntp.Pack()
      file := self.store.CreateTempFile(nntp.MessageID())
      if file != nil {
        err := self.store.WriteMessage(nntp, file)
        file.Close()
        if err == nil {
          self.infeed <- nntp
        } else {
          log.Println("failed to create startup messge?", err)
        }
      }
    }
  }()

  // get all pending articles from infeed and load them
  go func() {
    f, err := os.Open(self.store.TempDir()) 
    if err == nil {
      names, err := f.Readdirnames(0)
      if err == nil {
        for _, name := range names {
          self.infeed_load <- name
        }
      }
    }
    
  }()
  
  // if we have no frontend this does nothing
  if self.frontend != nil {
    go self.pollfrontend()
  }
  go self.pollinfeed()
  go self.pollmessages()  
  self.polloutfeeds()
}


func (self NNTPDaemon) pollfrontend() {
  chnl := self.frontend.NewPostsChan()
  for {
    nntp := <- chnl
    // new post from frontend
    log.Println("frontend post", nntp.MessageID())
    self.infeed <- nntp
  }
}
func (self NNTPDaemon) pollinfeed() {
  for {
    msgid := <- self.infeed_load
    log.Println("load from infeed", msgid)
    msg := self.store.ReadTempMessage(msgid)
    if msg != nil {
      self.infeed <- msg
    }
  }
}

func (self NNTPDaemon) polloutfeeds() {
  
  for {
    select {

    case outfeed := <- self.register_outfeed:
      log.Println("outfeed", outfeed.name, "registered")
      self.feeds[outfeed.name] = outfeed
    case outfeed := <- self.deregister_outfeed:
      log.Println("outfeed", outfeed.name, "de-registered")
      delete(self.feeds, outfeed.name)
    case nntp := <- self.send_all_feeds:
      feeds := self.feeds
      for _, feed := range feeds {
        if feed.policy.AllowsNewsgroup(nntp.Newsgroup()) {
          feed.check <- nntp.MessageID()
        }
      }
    case nntp := <- self.ask_for_article:
      for _, feed := range self.feeds {
        if feed.policy.AllowsNewsgroup(nntp.Newsgroup()) {
          log.Println("asking", feed.name, "for", nntp.MessageID())
          feed.article <- nntp.MessageID()
        }
      }
    }
  }
}

func (self NNTPDaemon) pollmessages() {
  var chnl chan NNTPMessage
  modchnl := self.mod.MessageChan()
  if self.frontend != nil {
    chnl = self.frontend.PostsChan()
  }
  for {
    
    nntp := <- self.infeed
    // ammend path
    nntp.AppendPath(self.instance_name)
    msgid := nntp.MessageID()
    log.Println("daemon got", msgid)
    
    // store article and attachments
    // register with database
    // this also generates thumbnails
    self.store.StorePost(nntp)
    
    // prepare for content rollover
    // fallback rollover
    rollover := 100
    
    group := nntp.Newsgroup()
    tpp, err := self.database.GetThreadsPerPage(group)
    ppb, err := self.database.GetPagesPerBoard(group)
    if err == nil {
      rollover = tpp * ppb
    }
    
    // roll over old content
    self.expire.ExpireGroup(group, rollover)
    // handle mod events
    if group == "ctl" {
      modchnl <- nntp
    }
    
    // queue to all outfeeds
    // XXX: blocking ?
    self.send_all_feeds <- ArticleEntry{msgid, group}
    // tell frontend
    // XXX: blocking ?
    if chnl != nil {
      if self.frontend.AllowNewsgroup(group) {
        chnl <- nntp
      }
    }
  }
}


func (self NNTPDaemon) acceptloop() {	
  for {
    // accept
    conn, err := self.listener.Accept()
    if err != nil {
      log.Fatal(err)
    }
    // make a new inbound nntp connection handler 
    nntp := createNNTPConnection()
    c := textproto.NewConn(conn)
    // send banners and shit
    err = nntp.inboundHandshake(c)
    if err == nil {
      // run, we support stream and reader
      go nntp.runConnection(self, true, true, true, c)
    } else {
      log.Println("failed to send banners", err)
      c.Close()
    }
  }
}

func (self NNTPDaemon) Setup() NNTPDaemon {
  log.Println("checking for configs...")
  // check that are configs exist
  CheckConfig()
  log.Println("loading config...")
  // read the config
  self.conf = ReadConfig()
  if self.conf == nil {
    log.Fatal("failed to load config")
  }
  // validate the config
  log.Println("validating configs...")
  self.conf.Validate()
  log.Println("configs are valid")

  
  db_host := self.conf.database["host"]
  db_port := self.conf.database["port"]
  db_user := self.conf.database["user"]
  db_passwd := self.conf.database["password"]

  // set up database stuff
  log.Println("connecting to database...")
  self.database = NewDatabase(self.conf.database["type"], self.conf.database["schema"], db_host, db_port, db_user, db_passwd)
  log.Println("ensure that the database is created...")
  self.database.CreateTables()

  // set up store
  log.Println("set up article store...")
  self.store = createArticleStore(self.conf.store, self.database)

  self.mod = modEngine{
    store: self.store,
    database:  self.database,
    chnl: make(chan NNTPMessage),
  }
  return self
}
