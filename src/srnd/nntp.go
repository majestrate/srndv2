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


type nntpStreamEvent string

func (ev nntpStreamEvent) MessageID() string {
  return strings.Split(string(ev), " ")[1]
}

func (ev nntpStreamEvent) Command() string {
  return strings.Split(string(ev), " ")[0]
}

func nntpTAKETHIS(msgid string) nntpStreamEvent {
  return nntpStreamEvent(fmt.Sprintf("TAKETHIS %s", msgid))
}

func nntpCHECK(msgid string) nntpStreamEvent {
  return nntpStreamEvent(fmt.Sprintf("CHECK %s", msgid))
}

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
  
  // ARTICLE <message-id>
  article chan string
  // TAKETHIS/CHECK <message-id>
  stream chan nntpStreamEvent
}

// write out a mime header to a writer
func writeMIMEHeader(wr io.Writer, hdr textproto.MIMEHeader) (err error) {
  // write headers
  for k, vals := range(hdr) {
    for _, val := range(vals) {
      _, err = io.WriteString(wr, fmt.Sprintf("%s: %s\n", k, val))
    }
  }
  // end of headers
  _, err = io.WriteString(wr, "\n")
  return
}

func createNNTPConnection() nntpConnection {
  return nntpConnection{
    article: make(chan string, 32),
    stream: make(chan nntpStreamEvent, 128),
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
    log.Println(self.name, line)
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
              if line == "MODE-READER\n" || line == "READER\n" {
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
  for err == nil {
    ev := <- self.stream
    log.Println(self.name, ev)
    if ValidMessageID(ev.MessageID()) {
      cmd , msgid := ev.Command(), ev.MessageID()
      if cmd == "TAKETHIS" {
        fname := daemon.store.GetFilename(msgid)
        if CheckFile(fname) {
          f, err := os.Open(fname)
          if err == nil {
            err = conn.PrintfLine("%s", ev)
            // time to send
            dw := conn.DotWriter()
            _ , err = io.Copy(dw, f)
            err = dw.Close()
            f.Close()
          }
        } else {
          log.Println(self.name, "didn't send", msgid, "we don't have it locally")
        }
      } else if cmd == "CHECK" {
        conn.PrintfLine("%s", ev)
      } else {
        log.Println("invalid stream command", ev)
      }
    }
  }
  return
}

// check if we want the article given its mime header
// returns empty string if it's okay otherwise an error message
func (self nntpConnection) checkMIMEHeader(daemon NNTPDaemon, hdr textproto.MIMEHeader) (reason string, err error) {

  newsgroup := hdr.Get("Newsgroups")
  reference := hdr.Get("References")
  msgid := hdr.Get("Message-Id")
  encaddr := hdr.Get("X-Encrypted-Ip")
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
    return
  } else if banned, _ := daemon.database.NewsgroupBanned(newsgroup) ; banned {
    reason = "newsgroup banned"
    return
  } else if ! ( ValidMessageID(msgid) || ( reference != "" && ! ValidMessageID(reference) ) ) {
    // invalid message id or reference
    reason = "invalid reference or message id is '" + msgid + "' reference is '"+reference + "'"
    return
  } else if daemon.database.ArticleBanned(msgid) {
    reason = "article banned"
  } else if reference != "" && daemon.database.ArticleBanned(reference) {
    reason = "thread banned"
  } else if daemon.database.HasArticleLocal(msgid) {
    // we already have this article locally
    reason = "have this article locally"
    return
  } else if daemon.database.HasArticle(msgid) {
    // we have already seen this article
    reason = "already seen"
    return
  } else if is_ctl {
    // we always allow control messages
    return 
  } else if anon_poster {
    // this was posted anonymously
    if daemon.allow_anon {
      if has_attachment || is_signed {
        // this is a signed message or has attachment
        if daemon.allow_anon_attachments {
          // we'll allow anon attachments
          return
        } else {
          // we don't take signed messages or attachments posted anonymously
          reason = "no anon signed posts or attachments"
          return
        }
      } else {
        // we allow anon posts that are plain
        return
      }
    } else {
      // we don't allow anon posts of any kind
      reason = "no anon posts allowed"
      return
    }
  } else {
    // check for banned address
    var banned bool
    if encaddr != "" {
      banned, err = daemon.database.CheckEncIPBanned(encaddr)
      if err == nil {
        if banned {
          // this address is banned
          reason = "address banned"
          return
        } else {
          // not banned
          return
        }
      }
    } else {
      // idk wtf
      log.Println(self.name, "wtf? invalid article")
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
      // send the article to us
      self.stream <- nntpTAKETHIS(msgid)
    }
    return
  } else if code == 239 {
    // successful TAKETHIS
    log.Println(msgid, "sent via", self.name)
    return
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
          // reader mode
          self.mode = "READER"
          log.Println(self.name, "switched to reader mode")
          conn.PrintfLine("201 No posting Permitted")
        } else if parts[1] == "STREAM" {
          // wut? we're already in streaming mode
          log.Println(self.name, "already in streaming mode")
          conn.PrintfLine("203 Streaming enabled brah")
        } else {
          // invalid
          log.Println(self.name, "got invalid mode request", parts[1])
          conn.PrintfLine("501 invalid mode variant:", parts[1])
        }
      } else if cmd == "QUIT" {
        // quit command
        conn.PrintfLine("")
        // close our connection and return
        conn.Close()
        return
      } else if cmd == "CHECK" {
        // handle check command
        msgid := parts[1]
        // have we seen this article?
        if daemon.database.HasArticle(msgid) {
          // yeh don't want it
          conn.PrintfLine("438 %s", msgid)
        } else if daemon.database.ArticleBanned(msgid) {
          // it's banned we don't want it
          conn.PrintfLine("438 %s", msgid)
        } else {
          // yes we do want it and we don't have it
          conn.PrintfLine("238 %s", msgid)
        }
      } else if cmd == "TAKETHIS" {
        // handle takethis command
        var hdr textproto.MIMEHeader
        var reason string
        // read the article header
        hdr, err = conn.ReadMIMEHeader()
        if err == nil {
          // check the header
          reason, err = self.checkMIMEHeader(daemon, hdr)
          dr := conn.DotReader()
          if len(reason) > 0 {
            // discard, we do not want
            code = 439
            log.Println(self.name, "rejected", msgid, reason)
            _, err = io.Copy(ioutil.Discard, dr)
            err = daemon.database.BanArticle(msgid, reason)
          } else {
            // check if we don't have the rootpost
            reference := hdr.Get("References")
            newsgroup := hdr.Get("Newsgroups")
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
              // write header
              err = writeMIMEHeader(f, hdr)
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
            code = 239
            reason = "gotten"
          }
        } else {
          log.Println(self.name, "error reading mime header:", err)
          code = 439
          reason = "error reading mime header"
        }
        conn.PrintfLine("%d %s %s", code, msgid, reason)
      } else if cmd == "ARTICLE" {
        if ValidMessageID(msgid) {
          if daemon.store.HasArticle(msgid) {
            // we have it yeh
            f, err := os.Open(daemon.store.GetFilename(msgid))
            if err == nil {
              conn.PrintfLine("220 %s", msgid)
              dw := conn.DotWriter()
              _, err = io.Copy(dw, f)
              dw.Close()
              f.Close()
            } else {
              // wtf?!
              conn.PrintfLine("503 idkwtf happened: %s", err.Error())
            }
          } else {
            // we dont got it
            conn.PrintfLine("430 %s", msgid)
          }
        } else {
          // invalid id
          conn.PrintfLine("500 Syntax error")
        }
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
    // next article to ask for
    msgid := <- self.article
    log.Println(self.name, "asking for", msgid)
    // send command
    conn.PrintfLine("ARTICLE %s", msgid)
    // read response
    code, line, err = conn.ReadCodeLine(-1)
    if code == 220 {
      // awwww yeh we got it
      var hdr textproto.MIMEHeader
      // read header
      hdr, err = conn.ReadMIMEHeader()
      if err == nil {
        // prepare to read body
        dr := conn.DotReader()
        // check header and decide if we want this
        reason, err := self.checkMIMEHeader(daemon, hdr)
        if err == nil {
          if len(reason) > 0 {
            log.Println(self.name, "discarding", msgid, reason)
            // we don't want it, discard
            io.Copy(ioutil.Discard, dr)
            daemon.database.BanArticle(msgid, reason)
          } else {
            // yeh we want it open up a file to store it in
            f := daemon.store.CreateTempFile(msgid)
            if f == nil {
              // already being loaded elsewhere
            } else {
              // write header to file
              writeMIMEHeader(f, hdr)
              // write article body to file
              _, _ = io.Copy(f, dr)
              // close file
              f.Close()
              log.Println(msgid, "obtained via reader from", self.name)
              // tell daemon to load article via infeed
              daemon.infeed_load <- msgid
            }
          }
        } else {
          // error happened while processing
          log.Println(self.name, "error happend while processing MIME header", err)
        }
      } else {
        // error happened while reading header
        log.Println(self.name, "error happened while reading MIME header", err)
      }
    } else if code == 430 {
      // they don't know it D:
      log.Println(msgid, "not known by", self.name)
    } else {
      // invalid response
      log.Println(self.name, "invald response to ARTICLE:", code, line)
    }
  }
  // report error and close connection
  log.Println(self.name, "error while in reader mode:", err)
  conn.Close()
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
        // no mode and inbound
        line, err = conn.ReadLine()
        log.Println(self.name, line)
        parts := strings.Split(line, " ")
        cmd := parts[0]
        if cmd == "CAPABILITIES" {
          // write capabilities
          conn.PrintfLine("101 i support to the following:")
          dw := conn.DotWriter()
          caps := []string{"VERSION 2", "READER", "STREAMING", "IMPLEMENTATION srndv2"}
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
          // handle a it as a command, we don't have a mode set
          parts := strings.Split(line, " ")
          var code64 int64
          code64, err = strconv.ParseInt(parts[0], 10, 32)
          if err ==  nil {
            err = self.handleLine(daemon, int(code64), line[4:], conn)
          } else {
            err = self.handleLine(daemon, 0, line, conn)
          }
        }
      } else { // no mode and outbound
        if preferMode == "stream" {
          // try outbound streaming
          if stream {
            success, err = self.modeSwitch("STREAM", conn)
            self.mode = "STREAM"
            if success {
              // start outbound streaming in background
              go self.startStreaming(daemon, reader, conn)
            }
          }
        } else if reader {
          // try reader mode
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
  log.Println(self.name, "got error", err)
  if ! inbound {
    // send quit on outbound
    conn.PrintfLine("QUIT")
  }
  conn.Close()
}

func (self nntpConnection) articleDefer(msgid string) {
  time.Sleep(time.Second * 90)
  self.stream <- nntpCHECK(msgid)
}
