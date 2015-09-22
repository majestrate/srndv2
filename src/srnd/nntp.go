//
// nntp.go -- nntp interface for peering
//
package srnd

import (
  "bufio"
  "fmt"
  "io"
  "io/ioutil"
  "log"
  "net/textproto"
  "os"
  "strconv"
  "strings"
  "sync"
  "time"
)

// nntp connection state
type nntpConnection struct {
  // the name of this feed
  name string
  // the mode we are in now
  mode string
  // the policy for federation
  policy FeedPolicy
  // lock help when expecting non pipelined activity
  access sync.Mutex
  
  // CHECK <message-id>
  check chan string
  // ARTICLE <message-id>
  article chan string
  // TAKETHIS <message-id>
  take chan string
}


func createNNTPConnection() nntpConnection {
  return nntpConnection{
    check: make(chan string, 32),
    article: make(chan string, 32),
    take: make(chan string, 32),
  }
}

// switch modes
func (self *nntpConnection) modeSwitch(mode string, conn *textproto.Conn) (success bool, err error) {
  self.access.Lock()
  mode = strings.ToUpper(mode)
  conn.PrintfLine("MODE %s", mode)
  log.Println("MODE", mode)
  var code int
  code, _, err = conn.ReadCodeLine(-1)
  if code > 200 && code < 300 {
    // accepted mode change
    if len(self.mode) > 0 {
      log.Printf("mode switch %s -> %s", self.mode, mode)
    } else {
      log.Println("switched to mode", mode)
    }
    self.mode = mode
    success = len(self.mode) > 0 
  }
  self.access.Unlock()
  return
}

// send a banner for inbound connections
func (self nntpConnection) inboundHandshake(conn *textproto.Conn) (err error) {
  err = conn.PrintfLine("200 Posting Allowed")
  return err
}

// outbound setup, check capabilities and set mode
// returns (supports stream, supports reader) + error
func (self nntpConnection) outboundHandshake(conn *textproto.Conn) (stream, reader bool, err error) {
  log.Println(self.name, "outbound handshake")
  var code int
  var line string
  for err == nil {
    code, line, err = conn.ReadCodeLine(-1)
    if err == nil {
      if code == 200 {
        // send capabilities
        log.Println(self.name, "ask for capabilities")
        err = conn.PrintfLine("CAPABILITIES")
        if err == nil {
          // read response
          dr := conn.DotReader()
          r := bufio.NewReader(dr)
          for {
            line, err = r.ReadString('\n')
            if err == io.EOF {
              // we are at the end of the dotreader
              // set err back to nil and break out
              err = nil
              break
            } else if err == nil {
              // we got a line
              if line == "MODE-READER\n" {
                log.Println(self.name, "supports READER")
                reader = true
              } else if line == "STREAMING\n" {
                stream = true
                log.Println(self.name, "supports STREAMING")
              } else if line == "POSTIHAVESTREAMING\n" {
                stream = true
                reader = false
                log.Println(self.name, "is SRNd")
              }
            } else {
              // we got an error
              log.Println("error reading capabilities", err)
              break
            }
          }
          // return after reading
          return
        }
      } else if code == 201 {
        log.Println("feed", self.name,"does not allow posting")
        // we don't do auth yet
        break
      } else {
        continue
      }
    }
  }
  return
}

// handle streaming event
// this function should send only
func (self nntpConnection) handleStreaming(daemon NNTPDaemon, reader bool, conn *textproto.Conn) (err error) {
  select {
  case msgid := <- self.check:
    log.Println(self.name, "CHECK", msgid)
    err = conn.PrintfLine("CHECK %s", msgid)
  case msgid := <- self.take:
    // send a file via TAKETHIS
    if ValidMessageID(msgid) {
      fname := daemon.store.GetFilename(msgid)
      if CheckFile(fname) {
        f, err := os.Open(fname)
        if err == nil {
          // time to send
          err = conn.PrintfLine("TAKETHIS %s", msgid)
          dw := conn.DotWriter()
          _ , err = io.Copy(dw, f)
          err = dw.Close()
          f.Close()
        }
      } else {
        log.Println(self.name, "didn't send", msgid, "we don't have it locally")
      }
    }
  }
  return
}

func (self nntpConnection) handleLine(daemon NNTPDaemon, code int, line string, conn *textproto.Conn) (err error) {
  parts := strings.Split(line, " ")
  var msgid string
  if code == 0 && len(parts) > 1 {
    msgid = parts[1]
  } else {
    msgid = parts[0]
  }
  if code == 238 {
    if ValidMessageID(msgid) {
      log.Println("sending", msgid, "to", self.name)
      // send the article to us
      self.take <- msgid
    }
  } else if code == 239 {
    // successful TAKETHIS
    log.Println(msgid, "sent via", self.name)
    // TODO: remember success 
  } else if code == 431 {
    // CHECK said we would like this article later
    log.Println("defer sending", msgid, "to", self.name)
    go self.articleDefer(msgid)
  } else if code == 439 {
    // TAKETHIS failed
    log.Println(msgid, "was not sent to", self.name, "denied:", line)
    // TODO: remember denial
  } else if code == 438 {
    // they don't want the article
    // TODO: remeber rejection
  } else {
    // handle command
    parts := strings.Split(line, " ")
    if len(parts) == 2 {
      cmd := parts[0]
      if cmd == "MODE" {
        if parts[1] == "READER" {
          self.mode = "READER"
          log.Println(self.name, "switched to reader mode")
          conn.PrintfLine("201 No posting Permitted")
          // handle reader mode
          self.startReader(daemon, conn)
        } else if parts[1] == "STREAM" {
          // wut? we're already in streaming mode
          log.Println(self.name, "already in streaming mode")
          conn.PrintfLine("203 Streaming enabled brah")
        } else {
          log.Println(self.name, "got invalid mode request", parts[1])
          conn.PrintfLine("501 invalid mode variant:", parts[1])
        }
      } else if cmd == "CHECK" {
        // handle check command
        msgid := parts[1]
        // have we seen this article?
        if daemon.database.HasArticle(msgid) {
          // yeh don't want it
          conn.PrintfLine("438 %s", msgid)
        } else {
          // yes we do want it and we don't have it
          conn.PrintfLine("238 %s", msgid)
        }
      } else if cmd == "TAKETHIS" {
        // read the article headers
        var hdr textproto.MIMEHeader
        var reason string
        hdr, err = conn.ReadMIMEHeader()
        if err == nil {
          // check the headers and see if we want this article
          newsgroup := hdr.Get("Newsgroups")
          reference := hdr.Get("References")
          msgid := hdr.Get("Message-ID")
          encaddr := hdr.Get("X-Encrypted-IP")
          torposter := hdr.Get("X-Tor-Poster")
          i2paddr := hdr.Get("X-I2p-Desthash")
          content_type := hdr.Get("Content-Type")
          has_attachment := strings.HasPrefix(content_type, "multipart/mixed")
          pubkey := hdr.Get("X-Pubkey-Ed25519")
          // TODO: allow certain pubkeys?
          is_signed := pubkey != ""
          is_ctl := newsgroup == "ctl" && is_signed
          anon_poster := torposter != "" || i2paddr != "" || encaddr == ""

          if ! newsgroupValidFormat(newsgroup) {
            // invalid newsgroup format
            reason = "invalid newsgroup"
            code = 439
          } else if ! ( ValidMessageID(msgid) || ( reference != "" && ! ValidMessageID(reference) ) ) {
            // invalid message id or reference
            reason = "invalid reference or message id is '" + msgid + "' reference is '"+reference + "'"
            code = 439
          } else if daemon.database.HasArticleLocal(msgid) {
            // we already have this article locally
            reason = "have this article locally"
            code = 439
          } else if daemon.database.HasArticle(msgid) {
            // we have already seen this article
            reason = "already seen"
            code = 439
          } else if is_ctl {
            // we always allow control messages
            code = 239
          } else if anon_poster {
            // this was posted anonymously
            if daemon.allow_anon {
              if has_attachment || is_signed {
                // this is a signed message or has attachment
                if daemon.allow_anon_attachments {
                  // we'll allow anon attachments
                  code = 239
                } else {
                  // we don't take signed messages or attachments posted anonymously
                  reason = "no anon signed posts or attachments"
                  code = 439
                }
              } else {
                // we allow anon posts that are plain
                code = 239
              }
            } else {
              // we don't allow anon posts of any kind
              reason = "no anon posts allowed"
              code = 439
            }
          } else {
            // check for banned address
            var banned bool
            if encaddr != "" {
              banned, err = daemon.database.CheckEncIPBanned(encaddr)
              if err == nil {
                if banned {
                // this address is banned
                  code = 439
                  reason = "address banned"
                } else {
                  // not banned
                  code = 239
                }
              } else {
                // an error occured
                log.Println(self.name, "failed to check ban for", encaddr, err)
                // do a 400
                conn.PrintfLine("400 Service temporarily unavailable")
                conn.Close()
                return
              }
            } else {
              // idk wtf
              log.Println(self.name, "wtf? invalid article")
            }
          }
          dr := conn.DotReader()
          // now read it
          if code == 239 {
            // we don't have this the rootpost
            if reference != "" && ValidMessageID(reference) && ! daemon.store.HasArticle(reference) && ! daemon.database.IsExpired(reference) {
              log.Println(self.name, "got reply to", reference, "but we don't have it")
              daemon.ask_for_article <- ArticleEntry{reference, newsgroup}
            }
            f := daemon.store.CreateTempFile(msgid)
            if f == nil {
              log.Println(self.name, "discarding", msgid, "we are already loading it")
              // discard
              io.Copy(ioutil.Discard, dr)
            } else {
              // write headers
              for k, vals := range(hdr) {
                for _, val := range(vals) {
                  _, err = io.WriteString(f, fmt.Sprintf("%s: %s\n", k, val))
                }
              }
              // end of headers
              _, err = io.WriteString(f, "\n")
              // write body
              _, err = io.Copy(f, dr)
              if err == nil || err == io.EOF {
                f.Close()
                // we gud, tell daemon
                daemon.infeed_load <- msgid
              } else {
                log.Println(self.name, "error reading message", err)
              }
            }
          } else {
            // discard
            log.Println(self.name, "rejected", msgid, reason)
            _, err = io.Copy(ioutil.Discard, dr)
          }
        } else {
          log.Println(self.name, "error reading mime header:", err)
        }
        if reason == "" {
          reason = "gotten"
        }
        conn.PrintfLine("%d %s %s", code, msgid, reason)
      }
    }
  }
  return
}

func (self nntpConnection) startStreaming(daemon NNTPDaemon, reader bool, conn *textproto.Conn) {
  var err error
  for err == nil {
    err = self.handleStreaming(daemon, reader, conn)
  }
  log.Println(self.name, "error while streaming:", err)
}

func (self nntpConnection) startReader(daemon NNTPDaemon, conn *textproto.Conn) {
  log.Println(self.name, "run reader mode")
  var err error
  var code int
  var line string
  for err == nil {
    msgid := <- self.article
    log.Println(self.name, "asking for", msgid)
    conn.PrintfLine("ARTICLE %s", msgid)
    code, line, err = conn.ReadCodeLine(-1)
    if code == 220 {
      // awwww yeh we got it
      f := daemon.store.CreateTempFile(msgid)
      if f == nil {
        // already being loaded elsewhere
      } else {
        // read in the article
        dr := conn.DotReader()
        _, err = io.Copy(f, dr)
        f.Close()
        log.Println(msgid, "got from", self.name)
        // tell daemon to load infeed
        daemon.infeed_load <- msgid
      }
    } else if code == 430 {
          // they don't know it D:
      log.Println(msgid, "not known by", self.name)
    } else {
      // invalid response
      log.Println(self.name, "invald response to ARTICLE:", code, line)
    } 
  }
  log.Println(self.name, "error while streaming:", err)
}

// run the mainloop for this connection
// stream if true means they support streaming mode
// reader if true means they support reader mode
func (self nntpConnection) runConnection(daemon NNTPDaemon, inbound, stream, reader bool, preferMode string, conn *textproto.Conn) {

  var err error
  var line string
  var success bool

  for err == nil {
    if self.mode == "" {
      if inbound  {
        self.name = "inbound-feed"
        // no mode set and inbound
        line, err = conn.ReadLine()
        log.Println(self.name, line)
        parts := strings.Split(line, " ")
        cmd := parts[0]
        if cmd == "CAPABILITIES" {
          // write capabilities
          conn.PrintfLine("101 Capabilities list:")
          dw := conn.DotWriter()
          caps := []string{"VERSION 2", "MODE-READER", "STREAMING"}
          for _, cap := range caps {
            io.WriteString(dw, cap)
            io.WriteString(dw, "\n")
          }
          dw.Close()
          log.Println(self.name, "sent Capabilities")
        } else if cmd == "MODE" {
          if len(parts) == 2 {
            if parts[1] == "READER" {
              // set reader mode
              self.mode = "READER"
              // posting is not permitted with reader mode
              conn.PrintfLine("201 Posting not permitted")
            } else if parts[1] == "STREAM" {
              // set streaming mode
              conn.PrintfLine("203 Stream it brah")
              self.mode = "STREAM"
              log.Println(self.name, "streaming enabled")
              go self.startStreaming(daemon, reader, conn)
            }
          }
        } else {
          log.Println(self.name,"in mode", self.mode, "got invalid inbound line:", line)
          conn.PrintfLine("500 Syntax Error")
        }
      } else {
        if preferMode == "stream" {
          if stream {
            success, err = self.modeSwitch("STREAM", conn)
            self.mode = "STREAM"
            if success {
              go self.startStreaming(daemon, reader, conn)
            }
          }
        } else if reader {
          success, err = self.modeSwitch("READER", conn)
          if success {
            self.mode = "READER"
            self.startReader(daemon, conn)
          }
        }
        if success {
          log.Println(self.name, "mode set to", self.mode)
        } else {
          // bullshit
          // we can't do anything so we quit
          log.Println(self.name, "can't stream or read, wtf?")
          conn.PrintfLine("QUIT")
          conn.Close()
          return
        }
      }
    } else {
      // we have our mode set
      line, err = conn.ReadLine()
      if err == nil {
        parts := strings.Split(line, " ")
        var code64 int64
        code64, err = strconv.ParseInt(parts[0], 10, 32)
        if err ==  nil {
          err = self.handleLine(daemon, int(code64), line[4:], conn)
        } else {
          err = self.handleLine(daemon, 0, line, conn)
        }
      }
    }
  }
  log.Println("run connection got error", err)
  if ! inbound {
    conn.PrintfLine("QUIT")
  }
  conn.Close()
}

func (self nntpConnection) articleDefer(msgid string) {
  time.Sleep(time.Second * 90)
  self.check <- msgid
}
