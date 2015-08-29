//
// nntp.go
//
package srnd

import (
  "bufio"
  "io"
  "io/ioutil"
  "log"
  "net"
  "net/textproto"
  "os"
  "strings"
  "sync"
  "time"
)
  
type ConnectionInfo struct {
  mode string
  newsgroup string
  allowsPosting bool 
  supportsStream bool
  state string
  // locked when we are reading or writing
  access sync.Mutex
}

type NNTPConnection struct {
  conn net.Conn
  txtconn *textproto.Conn 
  inbound bool
  debug bool
  info *ConnectionInfo
  policy *FeedPolicy
  // channel for senging sync messages
  sync chan ArticleEntry
  // message io
  msg_reader MessageReader
  msg_writer MessageWriter
  // do we allow articles from tor?
  allow_tor bool
  allow_tor_attachments bool
}

// ask if they need this article in streaming mode
func (self *NNTPConnection) askSync(nntp ArticleEntry) {
  if self.policy != nil && ! self.policy.AllowsNewsgroup(nntp.Newsgroup()) {
    // don't sync it it's now allowed
    log.Println("!!! bug: asking to sync article in",nntp.Newsgroup(),"but it violates feed policy")
    return
  }
  self.info.access.Lock()
  self.txtconn.PrintfLine("CHECK %s", nntp.MessageID())
  self.info.access.Unlock()
}

func (self *NNTPConnection) HandleOutbound(d *NNTPDaemon, quarks map[string]string, mode string) {
  code, line, _ := self.txtconn.ReadCodeLine(-1)
  self.info.allowsPosting = code == 200
  if ! self.info.allowsPosting {
    log.Printf("outbound feed posting not allowed: %d %s", code, line)
    self.Quit()
    return
  }
  // TODO: autodetect quarks
  if quarks != nil {
    // check for twisted news server quark
    t, ok := quarks["twisted"]
    if ok {
      if t == "1" {
        // force into post mode
        self.info.mode = "post"
      }
    }
  }

  if self.info.mode == "" {
    // they allow posting
    // send capabilities command
    _ = self.txtconn.PrintfLine("CAPABILITIES")
    capreader := bufio.NewReader(self.txtconn.DotReader())
    
    // get capabilites
    for {
      line, err := capreader.ReadString('\n') 
      if err != nil {
        log.Println(err)
        break
      }
      line = strings.ToLower(line)
      if line == "streaming\n" {
        self.info.supportsStream = true
      } else if line == "postihavestreaming\n" {
        self.info.supportsStream = true
      } else {
        // wtf now?
      }
    }
  }
  if mode == "reader" {
    // this is for sending ARTICLE commands
    self.reader_mode(d)
  } else if mode == "stream" {
    // if they support streaming and allow posting continue
    if self.info.supportsStream && self.info.allowsPosting {
      self.streaming_mode(d)
    } else if self.info.mode == "post" {
      // we are forced into post mode
      self.post_mode(d)
    }
  } else {
    log.Println("!!! bug outfeed using unknown mode", mode, "!!!")
    self.Quit()
  }
}

func (self *NNTPConnection) reader_mode(d *NNTPDaemon) {
  for {
    nntp := <- self.sync
    if self.policy != nil && ! self.policy.AllowsNewsgroup(nntp.Newsgroup()) {
      log.Println("!!! bug: asking outfeed for article that violates policy for newsgroup",nntp.Newsgroup(), "!!!")
      continue
    }
    msgid := nntp.MessageID()
    if ValidMessageID(msgid) {
      if d.store.HasArticle(msgid) {
        log.Println("we already have", msgid, "will not ask feed")
        continue
      }
      self.txtconn.PrintfLine("ARTICLE %s", msgid)
      code, line, err := self.txtconn.ReadCodeLine(-1)
      if code == 430 {
        // they don't have it D:
        continue // TODO: back off?
      } else if code == 230 {
        // they have it aww yehhh
        dr := self.txtconn.DotReader()
        w := d.store.CreateTempFile(msgid)
        if w == nil {
          // discard artilce, bug
          log.Println("!!! bug: disarding message", msgid, "!!!")
          io.Copy(ioutil.Discard, dr)
        } else {
          _, err = io.Copy(w, dr)
          w.Close()
          // tell infeed to load this
          d.infeed_load <- msgid
        }
      } else {
        log.Println("invalid outfeed response for reader mode:", code, line)
      }
      if err != nil {
        log.Println("error while outbound feed in reader mode", err)
        self.Quit()
        return
      }
    } else {
      log.Println("!!! bug: tried to ask for an invalid message id", msgid, "!!!")
    }
  }
}

// enter posting mode
func (self *NNTPConnection) post_mode(d *NNTPDaemon) {
  // TODO: improve this, it's dumb right now
  for {
    nntp := <- self.sync
    fname := d.store.GetFilename(nntp.MessageID())
    f, err := os.Open(fname)
    if f == nil {
      continue
    } 
    self.txtconn.PrintfLine("POST")

    _ , _, err = self.txtconn.ReadCodeLine(340)
    if err == nil {
      w := self.txtconn.DotWriter()
      _, err = io.Copy(w, f)
      w.Close()
    }
    f.Close()
  }
}

func (self *NNTPConnection) articleDefered(nntp ArticleEntry) {
  waittime := time.Second * 60
  log.Println(nntp.MessageID(), "was deferred, will try sending in",waittime)
  time.Sleep(waittime)
  self.sync <- nntp
}

// enter streaming mode
func (self *NNTPConnection) streaming_mode(d *NNTPDaemon) {
  var err error
  var line string
  var code int
  err = self.txtconn.PrintfLine("MODE STREAM")
  if err != nil {
    log.Println("failed to initiated streaming mode on feed", err)
    self.Close()
    return 	
  }
  code, line, err = self.txtconn.ReadCodeLine(-1)
  if err != nil {
    log.Println("failed to read response for streaming handshake on feed", err)
    self.Close()
    return
  }
  if code == 203 {
    self.info.mode = "stream"
    log.Println("streaming mode activated")
  } else {
    log.Println("streaming mode not activated, quitting")
    self.Quit()
    return
  }
  // go routine for sending sync requests
  go func() {
    for {
      nntp := <- self.sync
      if ValidMessageID(nntp.MessageID()) {
        self.askSync(nntp)
      }
    }
  }()
  
  for {
    code, line, err = self.txtconn.ReadCodeLine(-1)
    if err != nil {
      log.Println("error reading response code", err)
      self.Close()
      return
    }
    code = int(code)
    commands := strings.Split(line, " ")
    if code == 238 && len(commands) > 1 && ValidMessageID(commands[0]) {
      if d.store.HasArticle(commands[0]) {
        fname := d.store.GetFilename(commands[0])
        f, err := os.Open(fname)
        if err == nil {
          err = self.SendMessage(commands[0], f, d)
          f.Close()
          continue
        } 
      } else {
        log.Println("we didn't send", commands[0], "we don't have it locally")
        continue
      }
      log.Println("failed to send", commands[0], err)
    } else if code == 400 {
      // deferred
      // send it later
      nntp, err := d.database.GetMessageIDByHash(HashMessageID(commands[0]))
      if err == nil {
        go self.articleDefered(nntp)
      } else {
        log.Println("!!! bug: cannot defer article send", err, "!!!")
      }
      continue
    } else if code == 239 {
      // accepted
      continue
    } else if code == 439 || code == 438 {
      // invalid
      continue
    } else {
      log.Printf("invalid response from outbound feed: '%d %s'", code, line)
    }
  }
}

// just do it (tm)
func (self *NNTPConnection) SendMessage(msgid string, msg io.Reader, d *NNTPDaemon) error {
  var err error
  self.info.access.Lock()
  err = self.txtconn.PrintfLine("TAKETHIS %s", msgid)
  if err == nil {
    wr := self.txtconn.DotWriter()
    _, err = io.Copy(wr, msg)
    wr.Close()
  }
  self.info.access.Unlock()
  return err
}

func (self *NNTPConnection) ReadLine() (string, error) {
  b, err := self.txtconn.ReadLineBytes()
  var line string
  if err == nil {
    line = string(b)
    b = nil
  }
  return line, err
}

// handle inbound connection
func (self *NNTPConnection) HandleInbound(d *NNTPDaemon) {

  
  // intitiate handshake
  var err error
  self.info.mode = "stream"
  log.Println("Incoming nntp connection from", self.conn.RemoteAddr())
  // send welcome
  greet := "2nd generation overchan NNTP Daemon posting allowed"
  self.txtconn.PrintfLine("200 %s", greet)
  for {
    if err != nil {
      log.Println("failure in infeed", err)
      self.Close()
      return
    }
    // read line and break if needed
    line, err := self.ReadLine()
    if len(line) == 0 || err != nil {
      break
    }
    var code int
    var msg string
    commands := strings.Split(line, " ")
    cmd := commands[0]
    if cmd == "QUIT" {
      self.txtconn.PrintfLine("205 bai")
      break
    } else if cmd == "CAPABILITIES" { // capabilities command
      self.sendCapabilities()
    } else if cmd == "MODE" { // mode switch
      if len(commands) == 2 {
        // get mode
        mode := strings.ToUpper(commands[1])
        // reader mode
        if mode == "READER" {
          self.info.mode = "reader"
          code = 201
          msg = "posting disallowed"
        } else if mode == "STREAM" {
          // mode stream
          self.info.mode = "stream"
          code = 203
          msg = "stream it"
        } else {
          // other modes not implemented
          code = 501
          msg = "mode not implemented"
        }
      } else {
        code = 500
        msg = "syntax error"
      }
      
      self.txtconn.PrintfLine("%d %s", code, msg)
    } else if self.info.mode == "stream" { // we are in stream mode
      // clear reference
      line = ""
      if cmd == "TAKETHIS" {
        var newsgroup string
        if len(commands) == 2 && ValidMessageID(commands[1]) {
          article := commands[1]
          code := 239
          ip_header := ""
          ip_banned := false
          headers_done := false
          read_more := true
          has_attachment := false
          is_signed := false
          message := "we are gud"
          reference := ""
          file := d.store.CreateTempFile(article)
          if file == nil {
            code = 439
            message = "we have this message"
            read_more = false
          }
          for {
            if err != nil {
              log.Println("error reading", article, err)
              file.Close()
              fname := d.store.GetTempFilename(article)
              DelFile(fname)
              break
            }
            line , err := self.ReadLine()
            if err == nil && line == "" && ! headers_done {
              // headers done
              headers_done = true
              if len(ip_header) > 0 {
                ip_banned, err = d.database.CheckEncIPBanned(ip_header)
                if err == nil {
                  if ip_banned {
                    code = 439
                    read_more = false
                    message = "poster is banned"
                  }
                } else {
                  log.Println("cannot check for banned encrypted ip", err)
                  // send it later
                  code = 439
                  message = "could be banned by us but we do not know"
                }
              } else if self.allow_tor {
                // do we want tor posts with attachments?
                if has_attachment && ! self.allow_tor_attachments {
                  // this guy is banned
                  code = 439
                  read_more = false
                  message = "we do not take attachments from tor"
                }
              } else {
                // we don'e want it
                code = 439
                message = "we do not take anonymous posts"
                read_more = false
              }
              if is_signed {
                log.Println("we got a signed message")
              }
            } else if err != nil {
              if err == io.EOF {
                log.Println("unexpected EOF")
                self.Close()
                return
              }
              log.Println("error reading line", err)
              continue
            } else if line == "." {
              line = ""
              break
            } else if ! headers_done {
              lower_line := strings.ToLower(line)
              // newsgroup header
              // TODO: check feed policy if we allow this
              if strings.HasPrefix(lower_line, "newsgroups: ") {
                if len(newsgroup) == 0 {
                  newsgroup = line[12:]
                  if ! newsgroupValidFormat(newsgroup) {
                    // bad newsgroup
                    code = 439
                    message = "invalid newsgroup"
                    read_more = false
                  }
                }
              } else if strings.HasPrefix(lower_line, "x-tor-poster: 1") {
                if ! self.allow_tor {
                  // we don't want this post
                  code = 439
                  message = "we do not take anonymous posts"
                  read_more = false
                }
              } else if strings.HasPrefix(lower_line, "x-encrypted-ip: ") {
                ip_header = strings.Split(line, " ")[1]
                ip_header = strings.Trim(line, " \t\r\n")
              } else if strings.HasPrefix(lower_line, "content-type: multipart") {
                has_attachment = true
              } else if strings.HasPrefix(lower_line, "x-signature-ed25519-sha512: ") {
                is_signed = true
                has_attachment = true
              } else if strings.HasPrefix(lower_line, "references: ") {
                reference = strings.Trim(line[12:]," \t\r\n")
                if ValidMessageID(reference) {
                  if d.database.IsExpired(reference) {
                    code = 439
                    message = "this article belongs to an expired root post"
                    read_more = false
                  } else if ! d.database.HasArticleLocal(reference) {
                    // we don't have the root post yet
                    code = 400
                    message = "no root post yet, please send later"
                    read_more = false
                  }
                } else {
                  // invalid reference
                  code = 438
                  message = "this article has an invalid reference"
                  read_more = false
                }
              }
            }
            if read_more {
              file.Write([]byte(line))
              file.Write([]byte("\n")) 
            }
            line = ""
          }
          if file != nil {
            file.Close()
          }
          // tell them our result
          self.txtconn.PrintfLine("%d %s %s", code, article, message)
          // the send was good
          if code == 239 {
            log.Println(self.conn.RemoteAddr(), "got article", article)
            // inform daemon
            d.infeed_load <- article
          } else if code == 400 {
            // XXX: assumes that the reference is not in another newsgroup
            if reference == "" || newsgroup == "" {
              log.Println("invalid reference or newsgroup when defering article", reference, newsgroup)
            } else {
              log.Println(article, "was defered because we don't have",reference,"asking all feeds for it")
              d.ask_for_article <- ArticleEntry{reference, newsgroup}
            }
          } else {
            // delete unaccepted article
            log.Println("did not accept", article, code)
            fname := d.store.GetTempFilename(article)
            DelFile(fname)
          }
        } else {
          // discard
          dr := self.txtconn.DotReader()
          io.Copy(ioutil.Discard, dr)
          if len(commands) == 2 {
            self.txtconn.PrintfLine("439 %s invalid message-id", commands[1])
          } else {
            self.txtconn.PrintfLine("439 no message id")
          }
        }
      } else if cmd == "CHECK" {
        if len(commands) == 2 {
          // check syntax
          // send error if needed
          article := commands[1]
          if ! ValidMessageID(article) {
            self.txtconn.PrintfLine("501 bad message id")
            continue
          }
          // do we already know this article?
          if d.database.HasArticle(article) {
            // ya, we got it already
            // tell them to not send it
            self.txtconn.PrintfLine("438 %s we have this article", article)
          } else {
            // nope, we do not have it
            // tell them to send it
            self.txtconn.PrintfLine("238 %s we want this article please give it", article)
          }
        } else {
          // incorrect format for CHECK
          self.txtconn.PrintfLine("500 syntax error")
        }
      } else {
        // unknown command
        self.txtconn.PrintfLine("500 we don't know command %s", cmd)
      }
    } else if self.info.mode == "reader" {
      // reader mode
      if cmd == "ARTICLE" {
        // they requested an article
        if len(commands) == 2 {
          // requested via message id
          msgid := commands[1]
          if ValidMessageID(msgid) && d.store.HasArticle(msgid) {
            // we found it
            fname := d.store.GetFilename(msgid)
            f, err := os.Open(fname)
            if err == nil {
              // we opened it, send it
              self.txtconn.PrintfLine("220 0 %s", msgid)
              dw := self.txtconn.DotWriter()
              _, err = io.Copy(dw, f)
              dw.Close()
              f.Close()
              continue
            }
            log.Println("error fetching", msgid, err)
          }
          self.txtconn.PrintfLine("430 No Such Article Found")
        } else {
          // invalid syntax
          self.txtconn.PrintfLine("500 invalid syntax")
        }
      } else {
        // unhandled command in mode reader
        self.txtconn.PrintfLine("500 invalid command for reader:", cmd)
      }
    }
  }
  self.Close()
}

func (self *NNTPConnection) sendCapabilities() {
  wr := self.txtconn.DotWriter()
  io.WriteString(wr, "101 we can haz do things\r\n")
  io.WriteString(wr, "VERSION 2\r\n")
  io.WriteString(wr, "IMPLEMENTATION srndv2 better than SRNd\r\n")
  if self.info.mode == "reader" {
    io.WriteString(wr, "READER")
  } else {
    io.WriteString(wr, "STREAMING\r\n")
    io.WriteString(wr, "MODE-READER\r\n")
  }
  wr.Close()
}

func (self *NNTPConnection) Quit() {
  if self.inbound {
    log.Println("!!! bug, Quit() called on inbound connection !!! ")
  } else {
    self.txtconn.PrintfLine("QUIT")
    self.Close()
  }
}

// close the connection
func (self *NNTPConnection) Close() {
  err := self.conn.Close()
  if err != nil {
    log.Println(self.conn.RemoteAddr(), err)
  }
  log.Println(self.conn.RemoteAddr(), "Closed Connection")
}
