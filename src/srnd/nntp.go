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
  "time"
)
  
type ConnectionInfo struct {
  mode string
  newsgroup string
  allowsPosting bool 
  supportsStream bool
  state string
  // if true we are reading data
  reading bool
}

type NNTPConnection struct {
  conn net.Conn
  txtconn *textproto.Conn 
  inbound bool
  debug bool
  info *ConnectionInfo
  policy *FeedPolicy
  // channel for senging sync messages
  sync chan string
  // message io
  msg_reader MessageReader
  msg_writer MessageWriter
  // do we allow articles from tor?
  allow_tor bool
  allow_tor_attachments bool
}

// ask if they need this article
func (self *NNTPConnection) askSync(msgid string) {
  if ValidMessageID(msgid) {
    self.txtconn.PrintfLine("CHECK %s", msgid)
  }
}

func (self *NNTPConnection) HandleOutbound(d *NNTPDaemon, quarks map[string]string) {
  code, line, _ := self.txtconn.ReadCodeLine(-1)
  self.info.allowsPosting = code == 200
  if ! self.info.allowsPosting {
    log.Printf("outbound feed posting not allowed: %d %s", code, line)
    self.Close()
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
  // if they support streaming and allow posting continue
  // otherwise quit
  if self.info.supportsStream && self.info.allowsPosting {
    self.streaming_mode(d)
  } else if self.info.mode == "post" {
    // we are forced into post mode
    self.post_mode(d)
  }
}

// enter posting mode
func (self *NNTPConnection) post_mode(d *NNTPDaemon) {
  // TODO: improve this, it's dumb right now
  for {
    msg_id := <- self.sync
    fname := d.store.GetFilename(msg_id)
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


// enter streaming mode
func (self *NNTPConnection) streaming_mode(d *NNTPDaemon) {
  var err error
  var line string
  var code int
  err = self.txtconn.PrintfLine("MODE STREAM")
  if err != nil {
    log.Println("failed to initiated streaming mode on feed", err)
    return 	
  }
  code, line, err = self.txtconn.ReadCodeLine(-1)
  if err != nil {
    log.Println("failed to read response for streaming handshake on feed", err)
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
  log.Println("outfeed enter mainloop")

  // go routine for sending sync requests
  go func() {
    for {
      msg_id := <- self.sync
      for self.info.reading {
        time.Sleep(10 * time.Millisecond)
      }
      self.askSync(msg_id)
    }
  }()
  
  for {
    code, line, err = self.txtconn.ReadCodeLine(-1)
    if err != nil {
      log.Println("error reading response code", err)
      return
    }
    code = int(code)
    commands := strings.Split(line, " ")
    if code == 238 && len(commands) > 1 && ValidMessageID(commands[0]) {
      fname := d.store.GetFilename(commands[0])
      f, err := os.Open(fname)
      if f == nil {
        continue
      } 
      err = self.SendMessage(commands[0], f, d)
      f.Close()
      if err != nil {
        log.Println("failed to send message", err)
        self.Quit()
        return
      }
    } else if code == 438 {
      // declined
      continue
    } else if code == 239 {
      // accepted
      continue
    } else if code == 439 {
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
  self.info.reading = true
  err = self.txtconn.PrintfLine("TAKETHIS %s", msgid)
  if err != nil {
    log.Println("error in outfeed", err)
    return  err
  }
  wr := self.txtconn.DotWriter()
  _, err = io.Copy(wr, msg)
  wr.Close()
  self.info.reading = false
  if err != nil {
    log.Printf("failed to send %s via feed: %s", msgid, err)
    return err
  }
  return nil
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
      self.Quit()
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
    // capabilities command
    if cmd == "CAPABILITIES" {
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
          file := d.store.CreateTempFile(article)
          ip_header := ""
          ip_banned := false
          headers_done := false
          read_more := true
          has_attachment := false
          is_signed := false
          message := "we are gud"
          for {
            if err != nil {
              log.Println("error reading", article, err)
              break
            }
            line , err := self.ReadLine()
            if line == "" && ! headers_done {
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
                  code = 400
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
            }
            if line == "." {
              line = ""
              break
            } else if ! headers_done {
              lower_line := strings.ToLower(line)
              // newsgroup header
              // TODO: check feed policy if we allow this
              if strings.HasPrefix(lower_line, "newsgroups: ") {
                if len(newsgroup) == 0 {
                  newsgroup := line[12:]
                  if ! newsgroupValidFormat(newsgroup) {
                    // bad newsgroup
                    code = 439
                    message = "invalid newsgroup"
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
                reference := strings.Trim(line[12:]," \t\r\n")
                if d.database.IsExpired(reference) {
                  code = 439
                  message = "this article belongs to an expired root post"
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
          file.Close()
          // tell them our result
          self.txtconn.PrintfLine("%d %s %s", code, article, message)
          // the send was good
          if code == 239 {
            log.Println(self.conn.RemoteAddr(), "got article", article)
            // inform daemon
            d.infeed_load <- article
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
          // do we already have this article?
          if d.store.HasArticle(article) {
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
    }
  }
  self.Close()
}

func (self *NNTPConnection) sendCapabilities() {
  wr := self.txtconn.DotWriter()
  io.WriteString(wr, "101 we can haz do things\r\n")
  io.WriteString(wr, "VERSION 2\r\n")
  io.WriteString(wr, "IMPLEMENTATION srndv2 better than SRNd\r\n")
  io.WriteString(wr, "STREAMING\r\n")
  io.WriteString(wr, "READER\r\n")
  wr.Close()
}

func (self *NNTPConnection) Quit() {
  if ! self.inbound {
    self.txtconn.PrintfLine("QUIT")
  }
  self.Close()
}

// close the connection
func (self *NNTPConnection) Close() {
  err := self.conn.Close()
  if err != nil {
    log.Println(self.conn.RemoteAddr(), err)
  }
  log.Println(self.conn.RemoteAddr(), "Closed Connection")
}
