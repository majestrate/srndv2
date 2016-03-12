//
// daemon.go
//
package srnd

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// the state of a feed that we are persisting
type feedState struct {
	Config FeedConfig
	Paused bool
}

// the status of a feed that we are persisting
type feedStatus struct {
	// does this feed exist?
	Exists bool
	// the active connections this feed has open if it exists
	Conns []*nntpConnection
	// the state of this feed if it exists
	State *feedState
}

// an event for querying if a feed's status
type feedStatusQuery struct {
	// name of feed
	name string
	// channel to send result down
	resultChnl chan *feedStatus
}

// the result of modifying a feed
type modifyFeedPolicyResult struct {
	// error if one occured
	// set to nil if no error occured
	err error
	// name of the feed that was changed
	// XXX: is this needed?
	name string
}

// describes how we want to change a feed's policy
type modifyFeedPolicyEvent struct {
	// name of feed
	name string
	// new policy
	policy FeedPolicy
	// channel to send result down
	// if nil don't send result
	resultChnl chan *modifyFeedPolicyResult
}

type NNTPDaemon struct {
	instance_name string
	bind_addr     string
	conf          *SRNdConfig
	store         ArticleStore
	database      Database
	mod           ModEngine
	expire        ExpirationCore
	listener      net.Listener
	debug         bool
	sync_on_start bool
	// anon settings
	allow_anon             bool
	allow_anon_attachments bool

	// do we allow attachments from remote?
	allow_attachments bool

	running bool
	// http frontend
	frontend Frontend

	//cache driver
	cache CacheInterface

	// current feeds loaded from config
	loadedFeeds map[string]*feedState
	// for obtaining a list of loaded feeds from the daemon
	get_feeds chan chan []*feedStatus
	// for obtaining the status of a loaded feed
	get_feed chan *feedStatusQuery
	// for modifying feed's policies
	modify_feed_policy chan *modifyFeedPolicyEvent
	// for registering a new feed to persist
	register_feed chan FeedConfig
	// for degregistering an existing feed from persistance given name
	deregister_feed chan string
	// map of name -> NNTPConnection
	activeConnections map[string]*nntpConnection
	// for registering and deregistering outbound feed connections
	register_connection   chan *nntpConnection
	deregister_connection chan *nntpConnection
	// infeed for articles
	infeed chan NNTPMessage
	// channel to load messages to infeed given their message id
	infeed_load chan string
	// channel for broadcasting a message to all feeds given their newsgroup, message_id
	send_all_feeds chan ArticleEntry
	// channel for broadcasting an ARTICLE command to all feeds in reader mode
	ask_for_article chan ArticleEntry

	tls_config *tls.Config
}

func (self NNTPDaemon) End() {
	if self.listener != nil {
		self.listener.Close()
	}
	if self.database != nil {
		self.database.Close()
	}
	if self.cache != nil {
		self.cache.Close()
	}
}

func (self *NNTPDaemon) GetDatabase() Database {
	return self.database
}

// for srnd tool
func (self *NNTPDaemon) DelNNTPLogin(username string) {
	exists, err := self.database.CheckNNTPUserExists(username)
	if !exists {
		log.Println("user", username, "does not exist")
		return
	} else if err == nil {
		err = self.database.RemoveNNTPLogin(username)
	}
	if err == nil {
		log.Println("removed user", username)
	} else {
		log.Fatalf("error removing nntp login: %s", err.Error())
	}
}

// for srnd tool
func (self *NNTPDaemon) AddNNTPLogin(username, password string) {
	exists, err := self.database.CheckNNTPUserExists(username)
	if exists {
		log.Println("user", username, "exists")
		return
	} else if err == nil {
		err = self.database.AddNNTPLogin(username, password)
	}
	if err == nil {
		log.Println("added user", username)
	} else {
		log.Fatalf("error adding nntp login: %s", err.Error())
	}
}

func (self *NNTPDaemon) dialOut(proxy_type, proxy_addr, remote_addr string) (conn net.Conn, err error) {

	if proxy_type == "" || proxy_type == "none" {
		// connect out without proxy
		log.Println("dial out to ", remote_addr)
		conn, err = net.Dial("tcp", remote_addr)
		if err != nil {
			log.Println("cannot connect to outfeed", remote_addr, err)
			return
		}
	} else if proxy_type == "socks4a" || proxy_type == "socks" {
		// connect via socks4a
		log.Println("dial out via proxy", proxy_addr)
		conn, err = net.Dial("tcp", proxy_addr)
		if err != nil {
			log.Println("cannot connect to proxy", proxy_addr)
			return
		}
		// generate request
		idx := strings.LastIndex(remote_addr, ":")
		if idx == -1 {
			err = errors.New("invalid address: " + remote_addr)
			return
		}
		var port uint64
		addr := remote_addr[:idx]
		port, err = strconv.ParseUint(remote_addr[idx+1:], 10, 16)
		if port >= 25536 {
			err = errors.New("bad proxy port")
			return
		} else if err != nil {
			return
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

		log.Println("dial out via proxy", proxy_addr)
		conn, err = net.Dial("tcp", proxy_addr)
		// send request
		_, err = conn.Write(req)
		resp := make([]byte, 8)

		// receive response
		_, err = conn.Read(resp)
		if resp[1] == '\x5a' {
			// success
			log.Println("connected to", addr)
		} else {
			log.Println("failed to connect to", addr)
			conn.Close()
			conn = nil
			err = errors.New("failed to connect via proxy")
			return
		}
	} else {
		err = errors.New("invalid proxy type: " + proxy_type)
	}
	return
}

// save current feeds to feeds.ini, overwrites feeds.ini
// returns error if one occurs while writing to feeds.ini
func (self *NNTPDaemon) storeFeedsConfig() (err error) {
	feeds := self.activeFeeds()
	var feedconfigs []FeedConfig
	for _, status := range feeds {
		feedconfigs = append(feedconfigs, status.State.Config)
	}
	err = SaveFeeds(feedconfigs)
	return
}

// change a feed's policy given the feed's name
// return error if one occurs while modifying feed's policy
func (self *NNTPDaemon) modifyFeedPolicy(feedname string, policy FeedPolicy) (err error) {
	// make event
	chnl := make(chan *modifyFeedPolicyResult)
	ev := &modifyFeedPolicyEvent{
		resultChnl: chnl,
		name:       feedname,
		policy:     policy,
	}
	// fire event
	self.modify_feed_policy <- ev
	// recv result
	result := <-chnl
	if result == nil {
		// XXX: why would this ever happen?
		err = errors.New("no result from daemon after modifying feed")
	} else {
		err = result.err
	}
	// done with the event result channel
	close(chnl)
	return
}

// remove a persisted feed from the daemon
// does not modify feeds.ini
func (self *NNTPDaemon) removeFeed(feedname string) (err error) {
	// deregister feed first so it doesn't reconnect immediately
	self.deregister_feed <- feedname
	// deregister all connections for this feed
	status := self.getFeedStatus(feedname)
	for _, nntp := range status.Conns {
		go nntp.QuitAndWait()
	}
	return
}

func (self *NNTPDaemon) getFeedStatus(feedname string) (status *feedStatus) {
	chnl := make(chan *feedStatus)
	self.get_feed <- &feedStatusQuery{
		name:       feedname,
		resultChnl: chnl,
	}
	status = <-chnl
	close(chnl)
	return
}

// add a feed to be persisted by the daemon
// does not modify feeds.ini
func (self *NNTPDaemon) addFeed(conf FeedConfig) (err error) {
	self.register_feed <- conf
	return
}

// get an immutable list of all active feeds
func (self *NNTPDaemon) activeFeeds() (feeds []*feedStatus) {
	chnl := make(chan []*feedStatus)
	// query feeds
	self.get_feeds <- chnl
	// get reply
	feeds = <-chnl
	// got reply, close channel
	close(chnl)
	return
}

func (self *NNTPDaemon) persistFeed(conf FeedConfig, mode string) {
	log.Println(conf.Name, "persisting in", mode, "mode")
	backoff := time.Second
	for {
		if self.running {
			// get the status of this feed
			status := self.getFeedStatus(conf.Name)
			if !status.Exists {
				// our feed was removed
				// let's die
				log.Println(conf.Name, "ended", mode, "mode")
				return
			}
			if status.State.Paused {
				// we are paused
				// sleep for a bit
				time.Sleep(time.Second)
				// check status again
				continue
			}
			// do we want to do a pull based sync?

			if mode == "sync" {
				// yeh, do it
				self.syncPull(conf.proxy_type, conf.proxy_addr, conf.Addr)
				// sleep for the sleep interval and continue
				log.Println(conf.Name, "waiting for", conf.sync_interval, "before next sync")
				time.Sleep(conf.sync_interval)
				continue
			}
			conn, err := self.dialOut(conf.proxy_type, conf.proxy_addr, conf.Addr)
			if err != nil {
				log.Println(conf.Name, "failed to dial out", err.Error())
				log.Println(conf.Name, "back off for", backoff, "seconds")
				time.Sleep(backoff)
				// exponential backoff
				if backoff < (10 * time.Minute) {
					backoff *= 2
				}
				continue
			}
			nntp := createNNTPConnection(conf.Addr)
			nntp.policy = conf.policy
			nntp.feedname = conf.Name
			nntp.name = conf.Name + "-" + mode
			stream, reader, use_tls, err := nntp.outboundHandshake(textproto.NewConn(conn), &conf)
			if err == nil {
				if mode == "reader" && !reader {
					log.Println(nntp.name, "we don't support reader on this feed, dropping")
					conn.Close()
				} else {
					self.register_connection <- nntp
					// success connecting, reset backoff
					backoff = time.Second
					// run connection
					nntp.runConnection(self, false, stream, reader, use_tls, mode, conn, &conf)
					// deregister connection
					self.deregister_connection <- nntp
				}
			} else {
				log.Println("error doing outbound hanshake", err)
			}
		}
		log.Println(conf.Name, "back off for", backoff, "seconds")
		time.Sleep(backoff)
		// exponential backoff
		if backoff < (10 * time.Minute) {
			backoff *= 2
		}
	}
}

// do a oneshot pull based sync with another server
func (self *NNTPDaemon) syncPull(proxy_type, proxy_addr, remote_addr string) {
	c, err := self.dialOut(proxy_type, proxy_addr, remote_addr)
	if err == nil {
		conn := textproto.NewConn(c)
		// we connected
		nntp := createNNTPConnection(remote_addr)
		nntp.name = remote_addr + "-sync"
		// do handshake
		_, reader, _, err := nntp.outboundHandshake(conn, nil)

		if err != nil {
			log.Println("failed to scrape server", err)
		}
		if reader {
			// we can do it
			err = nntp.scrapeServer(self, conn)
			if err == nil {
				// we succeeded
				log.Println(nntp.name, "Scrape successful")
				nntp.Quit(conn)
				conn.Close()
			} else {
				// we failed
				log.Println(nntp.name, "scrape failed", err)
				conn.Close()
			}
		} else if err == nil {
			// we can't do it
			log.Println(nntp.name, "does not support reader mode, cancel scrape")
			nntp.Quit(conn)
		} else {
			// error happened
			log.Println(nntp.name, "error occurred when scraping", err)
		}
	}
}

// run daemon
func (self *NNTPDaemon) Run() {

	self.bind_addr = self.conf.daemon["bind"]

	listener, err := net.Listen("tcp", self.bind_addr)
	if err != nil {
		log.Fatal("failed to bind to", self.bind_addr, err)
	}
	self.listener = listener
	log.Printf("SRNd NNTPD bound at %s", listener.Addr())

	if self.conf.pprof != nil && self.conf.pprof.enable {
		addr := self.conf.pprof.bind
		log.Println("pprof enabled, binding to", addr)
		go func() {
			err := http.ListenAndServe(addr, nil)
			if err != nil {
				log.Fatalf("error from pprof, RIP srndv2: %s", err.Error())
			}
		}()
	}

	self.register_connection = make(chan *nntpConnection)
	self.deregister_connection = make(chan *nntpConnection)
	self.infeed = make(chan NNTPMessage, 10)
	self.infeed_load = make(chan string, 10)
	self.send_all_feeds = make(chan ArticleEntry, 10)
	self.activeConnections = make(map[string]*nntpConnection)
	self.loadedFeeds = make(map[string]*feedState)
	self.register_feed = make(chan FeedConfig)
	self.deregister_feed = make(chan string)
	self.get_feeds = make(chan chan []*feedStatus)
	self.get_feed = make(chan *feedStatusQuery)
	self.modify_feed_policy = make(chan *modifyFeedPolicyEvent)
	self.ask_for_article = make(chan ArticleEntry, 10)

	self.expire = createExpirationCore(self.database, self.store)
	self.sync_on_start = self.conf.daemon["sync_on_start"] == "1"
	self.instance_name = self.conf.daemon["instance_name"]
	self.allow_anon = self.conf.daemon["allow_anon"] == "1"
	self.allow_anon_attachments = self.conf.daemon["allow_anon_attachments"] == "1"
	self.allow_attachments = self.conf.daemon["allow_attachments"] == "1"

	// do we enable the frontend?
	if self.conf.frontend["enable"] == "1" {
		log.Printf("frontend %s enabled", self.conf.frontend["name"])

		cache_host := self.conf.cache["host"]
		cache_port := self.conf.cache["port"]
		cache_user := self.conf.cache["user"]
		cache_passwd := self.conf.cache["password"]
		self.cache = NewCache(self.conf.cache["type"], cache_host, cache_port, cache_user, cache_passwd, self.conf.frontend, self.database, self.store)

		self.frontend = NewHTTPFrontend(self, self.cache, self.conf.frontend, self.conf.worker["url"])
		go self.frontend.Mainloop()
	}

	// set up admin user if it's specified in the config
	pubkey, ok := self.conf.frontend["admin_key"]
	if ok {
		// TODO: check for valid format
		log.Println("add admin key", pubkey)
		err = self.database.MarkModPubkeyGlobal(pubkey)
		if err != nil {
			log.Printf("failed to add admin mod key, %s", err)
		}
	}

	log.Println("we have", len(self.conf.feeds), "feeds")

	defer self.listener.Close()
	// run expiration mainloop
	go self.expire.Mainloop()
	// we are now running
	self.running = true
	// start accepting incoming connections
	go self.acceptloop()

	go func() {
		// if we have no initial posts create one
		if self.database.ArticleCount() == 0 {
			nntp := newPlaintextArticle("welcome to nntpchan, this post was inserted on startup automatically", "system@"+self.instance_name, "Welcome to NNTPChan", "system", self.instance_name, genMessageID(self.instance_name), "overchan.test")
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
	go self.polloutfeeds()
	go self.pollmessages()
	// register feeds from config
	log.Println("registering feeds")
	for _, f := range self.conf.feeds {
		self.register_feed <- f
	}
	// if we want to sync on start do it now
	if self.sync_on_start {
		go func() {
			// wait 10 seconds for feeds to establish
			time.Sleep(10 * time.Second)
			self.syncAllMessages()
		}()
	}
	self.pollinfeed()
}

func (self *NNTPDaemon) syncAllMessages() {
	log.Println("syncing all messages to all feeds")
	for _, article := range self.database.GetAllArticles() {
		self.send_all_feeds <- article
	}
	log.Println("sync all messages queue flushed")
}

func (self *NNTPDaemon) pollinfeed() {
	for {
		msgid := <-self.infeed_load
		log.Println("load from infeed", msgid)
		msg := self.store.ReadTempMessage(msgid)
		if msg != nil {
			self.infeed <- msg
		}
	}
}

func (self *NNTPDaemon) polloutfeeds() {

	for {
		select {

		case q := <-self.get_feed:
			// someone asked for the status of a certain feed
			name := q.name
			// find feed
			feedstate, ok := self.loadedFeeds[name]
			if ok {
				// it exists
				if q.resultChnl != nil {
					// caller wants to be informed
					// create the reply
					status := &feedStatus{
						Exists: true,
						State:  feedstate,
					}
					// get the connections for this feed
					for _, conn := range self.activeConnections {
						if conn.feedname == name {
							status.Conns = append(status.Conns, conn)
						}
					}
					// tell caller
					q.resultChnl <- status
				}
			} else {
				// does not exist
				if q.resultChnl != nil {
					// tell caller
					q.resultChnl <- &feedStatus{
						Exists: false,
					}
				}
			}
		case ev := <-self.modify_feed_policy:
			// we want to modify a feed policy
			name := ev.name
			// does this feed exist?
			feedstate, ok := self.loadedFeeds[name]
			if ok {
				// yeh
				// replace the policy
				feedstate.Config.policy = ev.policy
				if ev.resultChnl != nil {
					// we need to inform the caller about the feed being changed successfully
					ev.resultChnl <- &modifyFeedPolicyResult{
						err:  nil,
						name: name,
					}
				}
			} else {
				// nah
				if ev.resultChnl != nil {
					// we need to inform the caller about the feed not existing
					ev.resultChnl <- &modifyFeedPolicyResult{
						err:  errors.New("no such feed"),
						name: name,
					}
				}
			}
		case chnl := <-self.get_feeds:
			// we got a request for viewing the status of the feeds
			var feeds []*feedStatus
			for feedname, feedstate := range self.loadedFeeds {
				var conns []*nntpConnection
				// get connections for this feed
				for _, conn := range self.activeConnections {
					if conn.feedname == feedname {
						conns = append(conns, conn)
					}
				}
				// add feedStatus
				feeds = append(feeds, &feedStatus{
					Exists: true,
					Conns:  conns,
					State:  feedstate,
				})
			}
			// send response
			chnl <- feeds
		case feedconfig := <-self.register_feed:
			self.loadedFeeds[feedconfig.Name] = &feedState{
				Config: feedconfig,
				// TODO: make starting paused configurable
				Paused: false,
			}
			log.Println("daemon registered feed", feedconfig.Name)
			// persist feeds
			if feedconfig.sync {
				go self.persistFeed(feedconfig, "sync")
			}
			go self.persistFeed(feedconfig, "stream")
			go self.persistFeed(feedconfig, "reader")
		case feedname := <-self.deregister_feed:
			_, ok := self.loadedFeeds[feedname]
			if ok {
				delete(self.loadedFeeds, feedname)
				log.Println("daemon deregistered feed", feedname)
			} else {
				log.Println("daemon does not have registered feed", feedname)
			}
		case outfeed := <-self.register_connection:
			self.activeConnections[outfeed.name] = outfeed
		case outfeed := <-self.deregister_connection:
			delete(self.activeConnections, outfeed.name)
		case nntp := <-self.send_all_feeds:
			group := nntp.Newsgroup()
			if self.Federate() {
				feeds := self.activeConnections
				for _, feed := range feeds {
					if feed.policy.AllowsNewsgroup(group) {
						if strings.HasSuffix(feed.name, "-stream") {
							msgid := nntp.MessageID()
							feed.offerStream(msgid)
						}
					}
				}
			}
		case nntp := <-self.ask_for_article:
			feeds := self.activeConnections
			for _, feed := range feeds {
				if feed.policy.AllowsNewsgroup(nntp.Newsgroup()) {
					if strings.HasSuffix(feed.name, "-reader") {
						log.Println("asking", feed.name, "for", nntp.MessageID())
						feed.article <- nntp.MessageID()
					}
				}
			}
		}
	}
}

func (self *NNTPDaemon) pollmessages() {

	modchnl := self.mod.MessageChan()
	for {

		nntp := <-self.infeed
		// ammend path
		nntp.AppendPath(self.instance_name)
		msgid := nntp.MessageID()
		log.Println("daemon got", msgid)

		ref := nntp.Reference()
		if ref != "" && ValidMessageID(ref) && !self.database.HasArticleLocal(ref) {
			// we don't have the root post
			// generate it
			//log.Println("creating temp root post for", ref , "in", nntp.Newsgroup())
			//root := newPlaintextArticle("temporary placeholder", "lol@lol", "root post "+ref+" not found", "system", "temp", ref, nntp.Newsgroup())
			//self.store.StorePost(root)
		}

		// prepare for content rollover
		// fallback rollover
		rollover := 100

		group := nntp.Newsgroup()
		tpp, err := self.database.GetThreadsPerPage(group)
		ppb, err := self.database.GetPagesPerBoard(group)
		if err == nil {
			rollover = tpp * ppb
		}

		// store article and attachments
		// register with database
		// this also generates thumbnails
		self.store.StorePost(nntp)
		// roll over old content
		self.expire.ExpireGroup(group, rollover)
		// queue to all outfeeds
		self.send_all_feeds <- ArticleEntry{msgid, group}
		// send to upper layers
		if group == "ctl" {
			modchnl <- msgid
		}
		if self.frontend != nil {
			if self.frontend.AllowNewsgroup(group) {
				self.frontend.PostsChan() <- frontendPost{msgid, ref, group}
			} else {
				log.Println("frontend does not allow", group, "not sending")
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
		hostname := ""
		if self.conf.crypto != nil {
			hostname = self.conf.crypto.hostname
		}
		nntp := createNNTPConnection(hostname)
		addr := conn.RemoteAddr()
		nntp.name = fmt.Sprintf("%s-inbound-feed", addr.String())
		c := textproto.NewConn(conn)
		// send banners and shit
		err = nntp.inboundHandshake(c)
		if err == nil {
			// run, we support stream and reader
			go nntp.runConnection(self, true, true, true, false, "stream", conn, nil)
		} else {
			log.Println("failed to send banners", err)
			c.Close()
		}
	}
}

func (self *NNTPDaemon) Federate() (federate bool) {
	federate = len(self.conf.feeds) > 0
	return
}

func (self *NNTPDaemon) GetOurTLSConfig() *tls.Config {
	return self.GetTLSConfig(self.conf.crypto.hostname)
}

func (self *NNTPDaemon) GetTLSConfig(hostname string) *tls.Config {
	cfg := self.tls_config
	return &tls.Config{
		ServerName:   hostname,
		CipherSuites: cfg.CipherSuites,
		RootCAs:      cfg.RootCAs,
		ClientCAs:    cfg.ClientCAs,
		Certificates: cfg.Certificates,
		ClientAuth:   cfg.ClientAuth,
	}
}

func (self *NNTPDaemon) RequireTLS() (require bool) {
	v, ok := self.conf.daemon["require_tls"]
	if ok {
		require = v == "1"
	}
	return
}

// return true if we can do tls
func (self *NNTPDaemon) CanTLS() (can bool) {
	can = self.tls_config != nil
	return
}

func (self *NNTPDaemon) Setup() {
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

	var err error

	log.Println("Reading translation files")
	translation_dir := self.conf.frontend["translations"]
	if translation_dir == "" {
		translation_dir = filepath.Join("contrib", "translations")
	}
	locale := self.conf.frontend["locale"]
	InitI18n(locale, translation_dir)

	db_host := self.conf.database["host"]
	db_port := self.conf.database["port"]
	db_user := self.conf.database["user"]
	db_passwd := self.conf.database["password"]

	// set up database stuff
	log.Println("connecting to database...")
	self.database = NewDatabase(self.conf.database["type"], self.conf.database["schema"], db_host, db_port, db_user, db_passwd)
	log.Println("ensure that the database is created...")
	self.database.CreateTables()

	// ensure tls stuff
	if self.conf.crypto != nil {
		self.tls_config, err = GenTLS(self.conf.crypto)
		if err != nil {
			log.Fatal("failed to initialize tls: ", err)
		}
	}

	// set up store
	log.Println("set up article store...")
	self.store = createArticleStore(self.conf.store, self.database)

	self.mod = modEngine{
		store:    self.store,
		database: self.database,
		chnl:     make(chan string, 1024),
	}
}
