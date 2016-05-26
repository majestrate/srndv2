package nntp

import (
	"crypto/tls"
	"encoding/json"
	log "github.com/Sirupsen/logrus"
	"github.com/majestrate/srndv2/lib/database"
	"net"
	"net/textproto"
	"strings"
)

// handles 1 line of input from a connection
type lineHandlerFunc func(c *v1Conn, line string) error

// base nntp connection
type v1Conn struct {
	// buffered connection
	C *textproto.Conn

	// unexported fields ...

	// connection state (mutable)
	state ConnState
	// state of tls connection
	tlsState *tls.ConnectionState
	// tls config for this connection, nil if we don't support tls
	tlsConfig *tls.Config
	// has this connection authenticated yet?
	authenticated bool
	// the username logged in with if it has authenticated via user/pass
	username string
	// underlying network socket
	conn net.Conn

	// database connection
	db database.DB

	// command handlers
	cmds map[string]lineHandlerFunc
}

// json representation of this connection
// format is:
// {
//   "state" : (connection state object),
//   "authed" : bool,
//   "tls" : (tls info or null if plaintext connection)
// }
func (c *v1Conn) MarshalJSON() ([]byte, error) {
	j := make(map[string]interface{})
	j["state"] = c.state
	j["authed"] = c.authenticated
	j["tls"] = c.tlsState
	return json.Marshal(j)
}

// get the current state of our connection (immutable)
func (c *v1Conn) GetState() (state *ConnState) {
	return &ConnState{
		FeedName: c.state.FeedName,
		ConnName: c.state.ConnName,
		HostName: c.state.HostName,
		Mode:     c.state.Mode,
		Group:    c.state.Group,
		Article:  c.state.Article,
		Policy: &FeedPolicy{
			Whitelist:            c.state.Policy.Whitelist,
			Blacklist:            c.state.Policy.Blacklist,
			AllowAnonPosts:       c.state.Policy.AllowAnonPosts,
			AllowAnonAttachments: c.state.Policy.AllowAnonAttachments,
			AllowAttachments:     c.state.Policy.AllowAttachments,
			UntrustedRequiresPoW: c.state.Policy.UntrustedRequiresPoW,
		},
	}
}

func (c *v1Conn) IsOpen() bool {
	return c.state.Open
}

func (c *v1Conn) Mode() Mode {
	return c.state.Mode
}

// is posting allowed rignt now?
func (c *v1Conn) PostingAllowed() bool {
	return c.authenticated
}

type v1RemoteConn struct {
	C v1Conn
}

// create a new connection from an established connection
func newOutboundConn(c net.Conn) *v1RemoteConn {
	return &v1RemoteConn{
		C: v1Conn{
			C:    textproto.NewConn(c),
			conn: c,
		},
	}
}

type v1IBConn struct {
	C v1Conn
}

func (c *v1IBConn) GetState() *ConnState {
	return c.C.GetState()
}

// negotiate an inbound connection
func (c *v1IBConn) Negotiate() (err error) {
	var line string
	if c.PostingAllowed() {
		line = Line_PostingAllowed
	} else {
		line = Line_PostingNotAllowed
	}
	err = c.C.printfLine(line)
	return
}

func (c *v1IBConn) PostingAllowed() bool {
	return c.C.PostingAllowed()
}

func (c *v1IBConn) IsOpen() bool {
	return c.C.IsOpen()
}

func (c *v1IBConn) Quit() {
	// inbound connections quit without warning
	log.WithFields(log.Fields{
		"pkg":  "nntp-ibconn",
		"addr": c.C.conn.RemoteAddr(),
	}).Info("closing inbound connection")
	c.C.conn.Close()
}

func (c *v1IBConn) WantsStreaming() bool {
	return c.C.state.Mode.Is(MODE_STREAM)
}

func (c *v1IBConn) ProcessInbound(filters []ArticleFilter, hooks EventHooks) {
	var err error
	var line string
	for err == nil {
		line, err = c.C.readline()
		if len(line) == 0 {
			// eof (proably?)
			c.Quit()
			return
		}

		uline := strings.ToUpper(line)
		parts := strings.Split(uline, " ")
		handler, ok := c.C.cmds[parts[0]]
		if ok {
			// we know the command
			err = handler(&c.C, line)
		} else {
			// we don't know the command
			err = c.C.printfLine("%s Unknown Command: %s", RPL_UnknownCommand, line)
		}
	}
}

func (c *v1Conn) printfLine(format string, args ...interface{}) error {
	log.WithFields(log.Fields{
		"pkg":     "nntp-conn",
		"version": 1,
		"state":   &c.state,
		"io":      "send",
	}).Debugf(format, args...)
	return c.C.PrintfLine(format, args...)
}

func (c *v1Conn) readline() (line string, err error) {
	line, err = c.C.ReadLine()
	log.WithFields(log.Fields{
		"pkg":     "nntp-conn",
		"version": 1,
		"state":   &c.state,
		"io":      "recv",
	}).Debug(line)
	return
}

// handle switching nntp modes for inbound connection
func switchModeInbound(c *v1Conn, line string) (err error) {
	cmd := ModeCommand(line)
	m := c.Mode()
	if cmd.Is(ModeReader) {
		if m.Is(MODE_STREAM) {
			// we need to stop streaming
		}
		var line string
		if c.PostingAllowed() {
			line = Line_PostingAllowed
		} else {
			line = Line_PostingNotAllowed
		}
		err = c.printfLine(line)
		if err == nil {
			c.state.Mode = MODE_READER
		}
	} else if cmd.Is(ModeStream) {
		// we want to switch to streaming mode
		err = c.printfLine(Line_StreamingAllowed)
		if err == nil {
			c.state.Mode = MODE_STREAM
		}
	} else {
		err = c.printfLine(Line_InvalidMode)
	}
	return
}

// handle quit command
func quitConnection(c *v1Conn, line string) (err error) {
	log.WithFields(log.Fields{
		"pkg":     "nntp-conn",
		"version": "1",
		"state":   &c.state,
	}).Debug("quit requested")
	_ = c.printfLine(Line_RPLQuit)
	err = c.C.Close()
	return
}

// send our capabailities
func sendCapabilities(c *v1Conn, line string) (err error) {
	var caps []string

	caps = append(caps, "MODE-READER", "IMPLEMENTATION nntpchand", "STREAMING")
	if c.tlsConfig != nil {
		caps = append(caps, "STARTTLS")
	}

	err = c.printfLine("%s We can do things", RPL_Capabilities)
	if err == nil {
		for _, l := range caps {
			err = c.printfLine(l)
			if err != nil {
				log.WithFields(log.Fields{
					"pkg":     "nntp-conn",
					"version": "1",
					"state":   &c.state,
				}).Error(err)
			}
		}
		err = c.printfLine(".")
	}
	return
}

// handle streaming line
func streamingLine(c *v1Conn, line string) (err error) {
	ev := StreamEvent(line)
	if c.Mode().Is(MODE_STREAM) {
		if ev.Valid() {
			// valid stream line
			cmd := ev.Command()
			msgid := ev.MessageID().String()
			if cmd == stream_CHECK {
				var has bool
				has, err = c.db.HasArticle(msgid)
				if err == nil {
					if has {
						// do have it, reject
						err = c.printfLine("%s %s", RPL_StreamingReject, msgid)
					} else {
						var banned bool
						banned, err = c.db.ArticleBanned(msgid)
						if banned {
							// it's banned, reject
							err = c.printfLine("%s %s", RPL_StreamingReject, msgid)
						} else {
							// don't have it and not banned, accept
							err = c.printfLine("%s %s", RPL_StreamingAccept, msgid)
						}
					}
				}
			}
		} else {
			// invalid line
			err = c.printfLine("%s Invalid syntax", RPL_SyntaxError)
		}
	} else {
		if ev.MessageID().Valid() {
			// not in streaming mode
			err = c.printfLine("%s %s", RPL_StreamingDefer, ev.MessageID())
		} else {
			// invalid message id
			err = c.printfLine("%s Invalid Syntax", RPL_SyntaxError)
		}
	}
	return
}

// inbound streaming start
func (c *v1IBConn) StartStreaming() (chnl chan ArticleEntry, send bool, err error) {
	if c.Mode().Is(MODE_STREAM) {
		chnl = make(chan ArticleEntry)
	} else {
		err = ErrInvalidMode
	}
	return
}

func (c *v1IBConn) Mode() Mode {
	return c.C.Mode()
}

// inbound streaming handling
func (c *v1IBConn) StreamAndQuit(policy ArticleAcceptor, filters []ArticleFilter, hooks EventHooks) {
	for {
		line, err := c.C.readline()
		if err == nil {
			// got line
			ev := StreamEvent(line)
			if ev.Valid() {
				msgid := ev.MessageID()
				if ev.Command() == stream_CHECK {
					if !msgid.Valid() {
						// invalid message id

					}
					// handle check command
					code := policy.CheckMessageID(msgid)
					var rpl StreamEvent
					if code.Accept() {
						rpl = stream_rpl_Accept(msgid)
					} else if code.Defer() {
						// defer it
						rpl = stream_rpl_Defer(msgid)
					} else {
						// disallowed
						rpl = stream_rpl_Reject(msgid)
					}
					err = c.C.printfLine("%s", rpl)
				}

				if c.Mode().Is(MODE_STREAM) {

				}
			} else {
				// invalid line?
				log.WithFields(log.Fields{
					"pkg":     "nntp-ibconn",
					"version": 1,
					"state":   &c.C.state,
					"line":    line,
				}).Error("invalid line")
			}
		} else {
			// readline failure
			log.WithFields(log.Fields{
				"pkg":     "nntp-ibconn",
				"version": 1,
				"state":   &c.C.state,
			}).Error("failure to read line during streaming")
			return
		}
	}
}

func newInboundConn(s *Server, c net.Conn) Conn {
	return &v1IBConn{
		C: v1Conn{
			db:   s.DB,
			C:    textproto.NewConn(c),
			conn: c,
			cmds: map[string]lineHandlerFunc{
				"MODE":         switchModeInbound,
				"QUIT":         quitConnection,
				"CAPABILITIES": sendCapabilities,
				"CHECK":        streamingLine,
				"TAKETHIS":     streamingLine,
			},
		},
	}
}
