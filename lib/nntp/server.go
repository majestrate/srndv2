package nntp

import (
	log "github.com/Sirupsen/logrus"
	"github.com/majestrate/srndv2/lib/config"
	"github.com/majestrate/srndv2/lib/database"
	"github.com/majestrate/srndv2/lib/network"
	"github.com/majestrate/srndv2/lib/store"
	"net"
	"time"
)

// callback hooks fired on certain events
type EventHooks interface {
	// called when we have obtained an article given its message-id
	GotArticle(msgid MessageID, group Newsgroup)
	// called when we have sent an article to a single remote feed
	SentArticleVia(msgid MessageID, feedname string)
}

// an nntp server
type Server struct {
	// user callbacks
	Hooks EventHooks
	// filters to apply
	Filters []ArticleFilter
	// database driver
	DB database.DB
	// global article acceptor
	Acceptor ArticleAcceptor
	// name of this server
	Name string
	// article storage
	Storage store.Storage
	// nntp config
	Config *config.NNTPServerConfig
	// outfeeds to connect to
	Feeds []*config.FeedConfig
}

func (s *Server) GotArticle(msgid MessageID, group Newsgroup) {
	log.WithFields(log.Fields{
		"pkg":   "nntp-server",
		"msgid": msgid,
		"group": group,
	}).Info("obtained article")
	if s.Hooks != nil {
		s.Hooks.GotArticle(msgid, group)
	}
}

func (s *Server) SentArticleVia(msgid MessageID, feedname string) {
	log.WithFields(log.Fields{
		"pkg":   "nntp-server",
		"msgid": msgid,
		"feed":  feedname,
	}).Info("article sent")
	if s.Hooks != nil {
		s.Hooks.SentArticleVia(msgid, feedname)
	}
}

// persist 1 feed forever
func (s *Server) persist(cfg *config.FeedConfig) {
	delay := time.Second

	log.WithFields(log.Fields{
		"name": cfg.Name,
	}).Debug("Persist Feed")
	for {
		dialer := network.NewDialer(cfg.Proxy)
		c, err := dialer.Dial(cfg.Addr)
		if err == nil {
			// successful connect
			delay = time.Second
			conn := newOutboundConn(c, s, cfg)
			err = conn.Negotiate()
			if err == nil {
				// negotiation good

			} else {
				log.WithFields(log.Fields{
					"name": cfg.Name,
				}).Info("outbound nntp connection failed to negotiate ", err)
				conn.Quit()
			}
		} else {
			// failed dial, do exponential backoff up to 1 hour
			if delay <= time.Hour {
				delay *= 2
			}
			log.WithFields(log.Fields{
				"name": cfg.Name,
			}).Info("feed backoff for ", delay)
			time.Sleep(delay)
		}
	}
}

// persist all outbound feeds
func (s *Server) PersistFeeds() {
	for _, f := range s.Feeds {
		go s.persist(f)
	}
}

// serve connections from listener
func (s *Server) Serve(l net.Listener) (err error) {
	log.WithFields(log.Fields{
		"pkg":  "nntp-server",
		"addr": l.Addr(),
	}).Debug("Serving")
	for err == nil {
		var c net.Conn
		c, err = l.Accept()
		if err == nil {
			// we got a new connection
			go s.handleInboundConnection(c)
		} else {
			log.WithFields(log.Fields{
				"pkg": "nntp-server",
			}).Error("failed to accept inbound connection", err)
		}
	}
	return
}

// get the article policy for a connection given its state
func (s *Server) getPolicyFor(state *ConnState) ArticleAcceptor {
	return s.Acceptor
}

// recv inbound streaming messages
func (s *Server) recvInboundStream(chnl chan ArticleEntry) {
	for {
		e, ok := <-chnl
		if ok {
			s.GotArticle(e.MessageID(), e.Newsgroup())
		} else {
			return
		}
	}
}

// process an inbound connection
func (s *Server) handleInboundConnection(c net.Conn) {
	log.WithFields(log.Fields{
		"pkg":  "nntp-server",
		"addr": c.RemoteAddr(),
	}).Debug("handling inbound connection")
	var nc Conn
	nc = newInboundConn(s, c)
	err := nc.Negotiate()
	if err == nil {
		// do they want to stream?
		if nc.WantsStreaming() {
			// yeeeeeh let's stream
			var chnl chan ArticleEntry
			chnl, err = nc.StartStreaming()
			// for inbound we will recv messages
			go s.recvInboundStream(chnl)
			nc.StreamAndQuit()
			log.WithFields(log.Fields{
				"pkg":  "nntp-server",
				"addr": c.RemoteAddr(),
			}).Info("streaming finished")
			return
		} else {
			// handle non streaming commands
			nc.ProcessInbound(s)
		}
	} else {
		log.WithFields(log.Fields{
			"pkg":  "nntp-server",
			"addr": c.RemoteAddr(),
		}).Warn("failed to negotiate with inbound connection", err)
		c.Close()
	}
}
