package nntp

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/majestrate/srndv2/lib/config"
	"github.com/majestrate/srndv2/lib/database"
	"github.com/majestrate/srndv2/lib/model"
	"github.com/majestrate/srndv2/lib/nntp/message"
	"github.com/majestrate/srndv2/lib/store"
	"github.com/majestrate/srndv2/lib/util"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/textproto"
	"strings"
)

// handles 1 line of input from a connection
type lineHandlerFunc func(c *v1Conn, line string, hooks EventHooks) error

// base nntp connection
type v1Conn struct {
	// buffered connection
	C *textproto.Conn

	// unexported fields ...

	// connection state (mutable)
	state ConnState
	// tls connection if tls is established
	tlsConn *tls.Conn
	// tls config for this connection, nil if we don't support tls
	tlsConfig *tls.Config
	// has this connection authenticated yet?
	authenticated bool
	// the username logged in with if it has authenticated via user/pass
	username string
	// underlying network socket
	conn net.Conn
	// server's name
	serverName string
	// article acceptor checks if we want articles
	acceptor ArticleAcceptor
	// headerIO for read/write of article header
	hdrio *message.HeaderIO
	// article storage
	storage store.Storage
	// database driver
	db database.DB
	// event callbacks
	hooks EventHooks
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
	if c.tlsConn == nil {
		j["tls"] = nil
	} else {
		j["tls"] = c.tlsConn.ConnectionState()
	}
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
	return c.Authed()
}

// process incoming commands
// call event hooks as needed
func (c *v1Conn) Process(hooks EventHooks) {
	var err error
	var line string
	for err == nil {
		line, err = c.readline()
		if len(line) == 0 {
			// eof (proably?)
			c.Close()
			return
		}

		uline := strings.ToUpper(line)
		parts := strings.Split(uline, " ")
		handler, ok := c.cmds[parts[0]]
		if ok {
			// we know the command
			err = handler(c, line, hooks)
		} else {
			// we don't know the command
			err = c.printfLine("%s Unknown Command: %s", RPL_UnknownCommand, line)
		}
	}
}

type v1OBConn struct {
	C               v1Conn
	supports_stream bool
	streamChnl      chan ArticleEntry
}

func (c *v1OBConn) IsOpen() bool {
	return c.IsOpen()
}

func (c *v1OBConn) Mode() Mode {
	return c.Mode()
}

func (c *v1OBConn) Negotiate() (err error) {
	return
}

func (c *v1OBConn) PostingAllowed() bool {
	return c.PostingAllowed()
}

func (c *v1OBConn) ProcessInbound(hooks EventHooks) {

}

func (c *v1OBConn) WantsStreaming() bool {
	return c.supports_stream
}

func (c *v1OBConn) StreamAndQuit() {
	for {
		e, ok := <-c.streamChnl
		if ok {
			// do CHECK
			msgid := e.MessageID()
			if !msgid.Valid() {
				log.WithFields(log.Fields{
					"pkg":   "nntp-conn",
					"state": c.C.state,
					"msgid": msgid,
				}).Warn("Dropping stream event with invalid message-id")
				continue
			}
			// send line
			err := c.C.printfLine("%s %s", stream_CHECK, msgid)
			if err == nil {
				// read response
				var line string
				line, err = c.C.readline()
				ev := StreamEvent(line)
				if ev.Valid() {
					cmd := ev.Command()
					if cmd == RPL_StreamingAccept {
						// accepted to send

						// check if we really have it in storage
						err = c.C.storage.HasArticle(msgid.String())
						if err == nil {
							var r io.ReadCloser
							r, err = c.C.storage.OpenArticle(msgid.String())
							if err == nil {
								log.WithFields(log.Fields{
									"pkg":   "nntp-conn",
									"state": c.C.state,
									"msgid": msgid,
								}).Debug("article accepted will send via TAKETHIS now")
								_ = c.C.printfLine("%s %s", stream_TAKETHIS, msgid)
								n, _ := io.Copy(c.C.C.W, r)
								r.Close()
								err = c.C.printfLine(".")
								if err == nil {
									// successful takethis sent
									log.WithFields(log.Fields{
										"pkg":   "nntp-conn",
										"state": c.C.state,
										"msgid": msgid,
										"bytes": n,
									}).Debug("article transfer done")
									// read response
									line, err = c.C.readline()
									ev := StreamEvent(line)
									if ev.Valid() {
										// valid reply
										cmd := ev.Command()
										if cmd == RPL_StreamingTransfered {
											// successful transfer
											log.WithFields(log.Fields{
												"feed":  c.C.state.FeedName,
												"msgid": msgid,
												"bytes": n,
											}).Debug("Article Transferred")
											// call hooks
											go c.C.hooks.SentArticleVia(msgid, c.C.state.FeedName)
										} else {
											// failed transfer
											log.WithFields(log.Fields{
												"feed":  c.C.state.FeedName,
												"msgid": msgid,
												"bytes": n,
											}).Debug("Article Rejected")
										}
									}
								}
							}
						} else {
							log.WithFields(log.Fields{
								"pkg":   "nntp-conn",
								"state": c.C.state,
								"msgid": msgid,
							}).Warn("article not in storage, not sending")
						}
					}
				} else {
					// invalid reply
					log.WithFields(log.Fields{
						"pkg":   "nntp-conn",
						"state": c.C.state,
						"msgid": msgid,
						"line":  line,
					}).Warn("invalid streaming response")
				}
			} else {
				log.WithFields(log.Fields{
					"pkg":   "nntp-conn",
					"state": c.C.state,
					"msgid": msgid,
				}).Error("streaming error during CHECK", err)
				return
			}
		} else {
			// channel closed
			return
		}
	}
}

func (c *v1OBConn) Quit() {
	c.C.printfLine("QUIT")
	c.C.readline()
	c.C.Close()
}

func (c *v1OBConn) StartStreaming() (chnl chan ArticleEntry, err error) {
	if c.streamChnl == nil {
		c.streamChnl = make(chan ArticleEntry)

	}
	chnl = c.streamChnl
	return
}

func (c *v1OBConn) GetState() *ConnState {
	return c.GetState()
}

// create a new connection from an established connection
func newOutboundConn(c net.Conn, s *Server, conf *config.FeedConfig) Conn {
	sname := s.Name
	if len(sname) == 0 {
		sname = "nntp.anon.tld"
	}
	storage := s.Storage
	if storage == nil {
		storage = store.NewNullStorage()
	}
	return &v1OBConn{
		C: v1Conn{
			state: ConnState{
				FeedName: conf.Name,
				HostName: conf.Addr,
				Open:     true,
			},
			serverName: sname,
			storage:    storage,
			C:          textproto.NewConn(c),
			conn:       c,
			hdrio:      message.NewHeaderIO(),
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
	c.C.Close()
}

// is this connection authenticated?
func (c *v1Conn) Authed() bool {
	return c.tlsConn != nil || c.authenticated
}

// unconditionally close connection
func (c *v1Conn) Close() {
	if c.tlsConn == nil {
		// tls is not on
		c.C.Close()
	} else {
		// tls is on
		// we should close tls cleanly
		c.tlsConn.Close()
	}
	c.state.Open = false
}

func (c *v1IBConn) WantsStreaming() bool {
	return c.C.state.Mode.Is(MODE_STREAM)
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
func switchModeInbound(c *v1Conn, line string, hooks EventHooks) (err error) {
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
func quitConnection(c *v1Conn, line string, hooks EventHooks) (err error) {
	log.WithFields(log.Fields{
		"pkg":     "nntp-conn",
		"version": "1",
		"state":   &c.state,
	}).Debug("quit requested")
	err = c.printfLine(Line_RPLQuit)
	c.Close()
	return
}

// send our capabailities
func sendCapabilities(c *v1Conn, line string, hooks EventHooks) (err error) {
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

// read an article via dotreader
func (c *v1Conn) readArticle(newpost bool, hooks EventHooks) (ps PolicyStatus, err error) {
	store_r, store_w := io.Pipe()
	article_r, article_w := io.Pipe()
	article_body_r, article_body_w := io.Pipe()

	accept_chnl := make(chan PolicyStatus)
	store_info_chnl := make(chan ArticleEntry)
	store_result_chl := make(chan error)

	hdr_chnl := make(chan message.Header)

	log.WithFields(log.Fields{
		"pkg": "nntp-conn",
	}).Debug("start reading")
	done_chnl := make(chan PolicyStatus)
	go func() {
		var err error
		br := c.C.R
		for err == nil {
			var line string
			line, err = br.ReadString(10)
			line = strings.Trim(line, "\r\n")
			if err == nil {
				if line == "." {
					// done
					break
				}
				line += "\n"
				_, err = io.WriteString(article_w, line)
			}
		}
		article_w.CloseWithError(err)
		st := <-accept_chnl
		close(accept_chnl)
		// get result from storage
		err2, ok := <-store_result_chl
		if ok {
			err = err2
		}
		close(store_result_chl)
		done_chnl <- st
	}()

	// parse message and store attachments in bg
	go func(msgbody io.ReadCloser) {
		defer msgbody.Close()
		hdr, ok := <-hdr_chnl
		if !ok {
			return
		}
		// all text in this post
		txt := new(bytes.Buffer)
		// the article itself
		a := new(model.Article)
		if hdr.IsMultipart() {
			_, params, err := hdr.GetMediaType()
			if err == nil {
				boundary, ok := params["boundary"]
				if ok {
					part_r := multipart.NewReader(msgbody, boundary)
					for {
						part, err := part_r.NextPart()
						if err == io.EOF {
							// we done
							break
						} else if err == nil {
							// we gots a part

							// get header
							part_hdr := part.Header

							// check for base64 encoding
							var part_body io.Reader
							if part_hdr.Get("Content-Transfer-Encoding") == "base64" {
								part_body = base64.NewDecoder(base64.StdEncoding, part)
							} else {
								part_body = part
							}

							// get content type
							content_type := part_hdr.Get("Content-Type")
							if len(content_type) > 0 {
								// assume text/plain
								content_type = "text/plain; charset=UTF8"
							}

							// extract mime type
							part_type, _, err := mime.ParseMediaType(content_type)
							if err == nil {

								if part_type == "text/plain" {
									// if we are plaintext save it to the text buffer
									_, err = io.Copy(txt, part_body)
								} else {
									var fpath string
									fname := part.FileName()
									fpath, err = c.storage.StoreAttachment(part_body, fname)
									if err == nil {
										// stored attachment good
										log.WithFields(log.Fields{
											"pkg":      "nntp-conn",
											"state":    &c.state,
											"version":  "1",
											"filename": fname,
											"filepath": fpath,
										}).Debug("attachment stored")
										a.Attachments = append(a.Attachments, model.Attachment{
											Path: fpath,
											Name: fname,
											Mime: part_type,
										})
									} else {
										// failed to save attachment
										log.WithFields(log.Fields{
											"pkg":     "nntp-conn",
											"state":   &c.state,
											"version": "1",
										}).Error("failed to save attachment ", err)
									}
								}
							} else {
								// cannot read part header
								log.WithFields(log.Fields{
									"pkg":     "nntp-conn",
									"state":   &c.state,
									"version": "1",
								}).Error("bad attachment in multipart message ", err)
							}
							part.Close()
						} else {
							// error reading part
							log.WithFields(log.Fields{
								"pkg":     "nntp-conn",
								"state":   &c.state,
								"version": "1",
							}).Error("error reading part ", err)
							break
						}
					}
				}
			}
		} else if hdr.IsSigned() {
			// signed message

			// discard for now
			_, err = io.Copy(util.Discard, msgbody)
		} else {
			// plaintext message
			var n int64
			n, err = io.Copy(txt, msgbody)
			log.WithFields(log.Fields{
				"bytes": n,
				"pkg":   "nntp-conn",
			}).Debug("text body copied")
		}
		if err == nil {
			// collect text
			a.Text = txt.String()
			log.Println("post text", a.Text)
		} else {
			log.WithFields(log.Fields{
				"pkg":   "nntp-conn",
				"state": &c.state,
			}).Error("error handing message body", err)
		}
	}(article_body_r)

	// store function
	go func(r io.ReadCloser) {
		e, ok := <-store_info_chnl
		if !ok {
			// failed to get info
			// don't read anything
			r.Close()
			return
		}
		msgid := e.MessageID()
		if msgid.Valid() {
			// valid message-id
			log.WithFields(log.Fields{
				"pkg":     "nntp-conn",
				"msgid":   msgid,
				"version": "1",
				"state":   &c.state,
			}).Debug("storing article")

			fpath, err := c.storage.StoreArticle(r, msgid.String())
			r.Close()
			if err == nil {
				log.WithFields(log.Fields{
					"pkg":     "nntp-conn",
					"msgid":   msgid,
					"version": "1",
					"state":   &c.state,
				}).Debug("stored article okay to ", fpath)
				// we got the article
				go hooks.GotArticle(msgid, e.Newsgroup())
				store_result_chl <- nil
			} else {
				// error storing article
				log.WithFields(log.Fields{
					"pkg":     "nntp-conn",
					"msgid":   msgid,
					"state":   &c.state,
					"version": "1",
				}).Error("failed to store article ", err)
				io.Copy(util.Discard, r)
				store_result_chl <- err
			}
		} else {
			// invalid message-id
			// discard
			log.WithFields(log.Fields{
				"pkg":     "nntp-conn",
				"msgid":   msgid,
				"state":   &c.state,
				"version": "1",
			}).Warn("store will discard message with invalid message-id")
			io.Copy(util.Discard, r)
			store_result_chl <- nil
			r.Close()
		}
	}(store_r)

	// acceptor function
	go func(r io.ReadCloser, out_w, body_w io.WriteCloser) {
		defer r.Close()
		status := PolicyAccept
		hdr, err := c.hdrio.ReadHeader(r)
		if err == nil {
			// get message-id
			var msgid MessageID
			if newpost {
				// new post
				// generate it
				msgid = GenMessageID(c.serverName)
			} else {
				// not a new post, get from header
				msgid = MessageID(hdr.MessageID())
				if msgid.Valid() {
					// check store fo existing article
					err = c.storage.HasArticle(msgid.String())
					if err == ErrArticleNotFound {
						// we don't have the article
						status = PolicyAccept
					} else if err == nil {
						// we do have the article, reject it we don't need it again
						status = PolicyReject
					} else {
						// some other error happened
						log.WithFields(log.Fields{
							"pkg":   "nntp-conn",
							"state": c.state,
						}).Error("failed to check store for article ", err)
					}
					err = nil
				} else {
					// bad article
					status = PolicyBan
				}
			}
			// check the header if we have an acceptor and the previous checks are good
			if status.Accept() && c.acceptor != nil {
				status = c.acceptor.CheckHeader(hdr)
			}
			// prepare to write header
			var w io.Writer
			if status.Accept() {
				// we have accepted the article
				// store to disk
				w = out_w
				// inform store
				store_info_chnl <- ArticleEntry{msgid.String(), hdr.Newsgroup()}
				hdr_chnl <- hdr
			} else {
				// we have not accepted the article
				// discard
				w = util.Discard
			}
			// close the channel for headers
			close(hdr_chnl)
			// write header out to storage
			err = c.hdrio.WriteHeader(hdr, w)
			if err == nil {
				mw := io.MultiWriter(body_w, w)
				// we wrote header
				var n int64
				if c.acceptor == nil {
					// write the rest of the body
					// we don't care about article size
					log.WithFields(log.Fields{}).Debug("copying body")
					for err == nil {
						var n2 int64
						n2, err = io.CopyN(mw, r, 128)
						n += n2
						log.Println(n2)
					}
				} else {
					// we care about the article size
					max := c.acceptor.MaxArticleSize()
					var n int64
					// copy it out
					n, err = io.CopyN(mw, r, max)
					if err == nil {
						if n < max {
							// under size limit
							// we gud
							log.WithFields(log.Fields{
								"pkg":   "nntp-conn",
								"bytes": n,
								"state": &c.state,
							}).Debug("body fits")
						} else {
							// too big, discard the rest
							_, err = io.Copy(util.Discard, r)
							// ... and ban it
							status = PolicyBan
						}
					}
				}
				log.WithFields(log.Fields{
					"pkg":   "nntp-conn",
					"bytes": n,
					"state": &c.state,
				}).Debug("body wrote")
				// TODO: inform store to delete article and attachments
			} else {
				// error writing header
				log.WithFields(log.Fields{
					"msgid": msgid,
				}).Error("error writing header ", err)
			}
		} else {
			// error reading header
			// possibly a read error?
			status = PolicyDefer
		}
		// close info channel for store
		close(store_info_chnl)
		out_w.Close()
		// close body pipe
		body_w.Close()
		// inform result
		accept_chnl <- status
	}(article_r, store_w, article_body_w)

	ps = <-done_chnl
	close(done_chnl)
	return
}

// handle IHAVE command
func nntpRecvArticle(c *v1Conn, line string, hooks EventHooks) (err error) {
	parts := strings.Split(line, " ")
	if len(parts) == 2 {
		msgid := MessageID(parts[1])
		if msgid.Valid() {
			// valid message-id
			err = c.printfLine("%s send article to be transfered", RPL_TransferAccepted)
			// read in article
			if err == nil {
				var status PolicyStatus
				status, err = c.readArticle(false, hooks)
				if err == nil {
					// we read in article
					if status.Accept() {
						// accepted
						err = c.printfLine("%s transfer wuz gud", RPL_TransferOkay)
					} else if status.Defer() {
						// deferred
						err = c.printfLine("%s transfer defer", RPL_TransferDefer)
					} else if status.Reject() {
						// rejected
						err = c.printfLine("%s transfer rejected, don't send it again brah", RPL_TransferReject)
					}
				} else {
					// could not transfer article
					err = c.printfLine("%s transfer failed; try again later", RPL_TransferDefer)
				}
			}
		} else {
			// invalid message-id
			err = c.printfLine("%s article not wanted", RPL_TransferNotWanted)
		}
	} else {
		// invaldi syntax
		err = c.printfLine("%s invalid syntax", RPL_SyntaxError)
	}
	return
}

// handle POST command
func nntpPostArticle(c *v1Conn, line string, hooks EventHooks) (err error) {
	if c.PostingAllowed() {
		if c.Mode().Is(MODE_READER) {
			err = c.printfLine("%s go ahead yo", RPL_PostAccepted)
			var status PolicyStatus
			status, err = c.readArticle(true, hooks)
			if err == nil {
				// read okay
				if status.Accept() {
					err = c.printfLine("%s post was recieved", RPL_PostReceived)
				} else {
					err = c.printfLine("%s posting failed", RPL_PostingFailed)
				}
			} else {
				log.WithFields(log.Fields{
					"pkg":     "nntp-conn",
					"state":   &c.state,
					"version": "1",
				}).Error("POST failed ", err)
				err = c.printfLine("%s post failed: %s", RPL_PostingFailed, err)
			}
		} else {
			// not in reader mode
			err = c.printfLine("%s not in reader mode", RPL_WrongMode)
		}
	} else {
		err = c.printfLine("%s posting is disallowed", RPL_PostingNotPermitted)
	}
	return
}

// handle streaming line
func streamingLine(c *v1Conn, line string, hooks EventHooks) (err error) {
	ev := StreamEvent(line)
	if c.Mode().Is(MODE_STREAM) {
		if ev.Valid() {
			// valid stream line
			cmd := ev.Command()
			msgid := ev.MessageID()
			if cmd == stream_CHECK {
				if c.acceptor == nil {
					// no acceptor, we'll take them all
					err = c.printfLine("%s %s", RPL_StreamingAccept, msgid)
				} else {
					status := PolicyAccept
					if c.storage.HasArticle(msgid.String()) == nil {
						// we have this article
						status = PolicyReject
					}
					if status.Accept() && c.acceptor != nil {
						status = c.acceptor.CheckMessageID(ev.MessageID())
					}
					if status.Accept() {
						// accepted
						err = c.printfLine("%s %s", RPL_StreamingAccept, msgid)
					} else if status.Defer() {
						// deferred
						err = c.printfLine("%s %s", RPL_StreamingDefer, msgid)
					} else {
						// rejected
						err = c.printfLine("%s %s", RPL_StreamingReject, msgid)
					}
				}
			} else if cmd == stream_TAKETHIS {
				var status PolicyStatus
				status, err = c.readArticle(false, hooks)
				if status.Accept() {
					// this article was accepted
					err = c.printfLine("%s %s", RPL_StreamingAccept, msgid)
				} else {
					// this article was not accepted
					err = c.printfLine("%s %s", RPL_StreamingReject, msgid)
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

func newsgroupList(c *v1Conn, line string, hooks EventHooks) (err error) {
	var groups []string
	if c.db == nil {
		// no database driver available
		// let's say we carry overchan.test for now
		groups = append(groups, "overchan.test")
	} else {
		groups, err = c.db.GetAllNewsgroups()
	}

	if err == nil {
		// we got newsgroups from the db
		dw := c.C.DotWriter()
		fmt.Fprintf(dw, "%s list of newsgroups follows\n", RPL_List)
		for _, g := range groups {
			hi := int64(1)
			lo := int64(0)
			if c.db != nil {
				hi, lo, err = c.db.GetLastAndFirstForGroup(g)
			}
			if err != nil {
				// log error if it occurs
				log.WithFields(log.Fields{
					"pkg":   "nntp-conn",
					"group": g,
					"state": c.state,
				}).Warn("cannot get high low water marks for LIST command")

			} else {
				fmt.Fprintf(dw, "%s %d %d y", g, hi, lo)
			}
		}
		// flush dotwriter
		err = dw.Close()
	} else {
		// db error while getting newsgroup list
		err = c.printfLine("%s cannot list newsgroups %s", RPL_GenericError, err.Error())
	}
	return
}

// handle inbound STARTTLS command
func upgradeTLS(c *v1Conn, line string, hooks EventHooks) (err error) {
	if c.tlsConfig == nil {
		err = c.printfLine("%s TLS not supported", RPL_TLSRejected)
	} else {
		err = c.printfLine("%s Continue with TLS Negotiation", RPL_TLSContinue)
		if err == nil {
			tconn := tls.Server(c.conn, c.tlsConfig)
			err = tconn.Handshake()
			if err == nil {
				// successful tls handshake
				c.tlsConn = tconn
				c.C = textproto.NewConn(c.tlsConn)
			} else {
				// tls failed
				log.WithFields(log.Fields{
					"pkg":   "nntp-conn",
					"addr":  c.conn.RemoteAddr(),
					"state": c.state,
				}).Warn("TLS Handshake failed ", err)
				// fall back to plaintext
				err = nil
			}
		}
	}
	return
}

// switch to another newsgroup
func switchNewsgroup(c *v1Conn, line string, hooks EventHooks) (err error) {
	parts := strings.Split(line, " ")
	var has bool
	var group Newsgroup
	if len(parts) == 2 {
		group = Newsgroup(parts[1])
		if group.Valid() {
			// correct format
			if c.db == nil {
				// no database driver
				has = true
			} else {
				has, err = c.db.HasNewsgroup(group.String())
			}
		}
	}
	if has {
		// we have it
		hi := int64(1)
		lo := int64(0)
		if c.db != nil {
			// check database for water marks
			hi, lo, err = c.db.GetLastAndFirstForGroup(group.String())
		}
		if err == nil {
			// XXX: ensure hi > lo
			err = c.printfLine("%s %d %d %d %s", RPL_Group, hi-lo, lo, hi, group.String())
			if err == nil {
				// line was sent
				c.state.Group = group
				log.WithFields(log.Fields{
					"pkg":   "nntp-conn",
					"group": group,
					"state": c.state,
				}).Debug("switched newsgroups")
			}
		} else {
			err = c.printfLine("%s error checking for newsgroup %s", RPL_GenericError, err.Error())
		}
	} else if err != nil {
		// error
		err = c.printfLine("%s error checking for newsgroup %s", RPL_GenericError, err.Error())
	} else {
		// incorrect format
		err = c.printfLine("%s no such newsgroup", RPL_NoSuchGroup)
	}
	return
}

// inbound streaming start
func (c *v1IBConn) StartStreaming() (chnl chan ArticleEntry, err error) {
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

func (c *v1IBConn) ProcessInbound(hooks EventHooks) {
	c.C.Process(hooks)
}

// inbound streaming handling
func (c *v1IBConn) StreamAndQuit() {
}

func newInboundConn(s *Server, c net.Conn) Conn {
	sname := s.Name
	if len(sname) == 0 {
		sname = "nntp.anon.tld"
	}
	storage := s.Storage
	if storage == nil {
		storage = store.NewNullStorage()
	}
	anon := false
	if s.Config != nil {
		anon = s.Config.AnonNNTP
	}
	return &v1IBConn{
		C: v1Conn{
			state: ConnState{
				FeedName: "inbound-feed",
				HostName: c.RemoteAddr().String(),
				Open:     true,
			},
			authenticated: anon,
			serverName:    sname,
			storage:       storage,
			acceptor:      s.Acceptor,
			hdrio:         message.NewHeaderIO(),
			C:             textproto.NewConn(c),
			conn:          c,
			cmds: map[string]lineHandlerFunc{
				"STARTTLS":     upgradeTLS,
				"IHAVE":        nntpRecvArticle,
				"POST":         nntpPostArticle,
				"MODE":         switchModeInbound,
				"QUIT":         quitConnection,
				"CAPABILITIES": sendCapabilities,
				"CHECK":        streamingLine,
				"TAKETHIS":     streamingLine,
				"LIST":         newsgroupList,
				"GROUP":        switchNewsgroup,
			},
		},
	}
}
