//
// nntp.go
//
package srnd

import (
  "bufio"
  "io"
  "log"
  "net"
  "net/textproto"
  "strings"
  "time"
)
  
type ConnectionInfo struct {
  mode string
  newsgroup string
  allowsPosting bool 
  supportsStream bool
  state string
}

type NNTPConnection struct {
  conn net.Conn
  txtconn *textproto.Conn 
  inbound bool
  debug bool
  info *ConnectionInfo
  policy *FeedPolicy
  send chan *NNTPMessage
  // channel for senging sync messages
  sync chan string
  // if true we are reading data
  reading bool
}

// ask if they need this article
func (self *NNTPConnection) askSync(msgid string) {
  self.txtconn.PrintfLine("CHECK %s", msgid)
}

func (self *NNTPConnection) HandleOutbound(d *NNTPDaemon) {
  var err error
  code, line, err := self.txtconn.ReadCodeLine(-1)
  self.info.allowsPosting = code == 200
  if ! self.info.allowsPosting {
    log.Printf("outbound feed posting not allowed: %d %s", code, line)
    self.Close()
    return
  }
  // they allow posting
  // send capabilities command
  err = self.txtconn.PrintfLine("CAPABILITIES")
  capreader := bufio.NewReader(self.txtconn.DotReader())
  
  // get capabilites
  for {
    line, err := capreader.ReadString('\n') 
    if err != nil {
      break
    }
    line = strings.ToLower(line)
    if line == "streaming\n" {
      self.info.supportsStream = true
    } else if line == "postihavestreaming\n" {
      self.info.supportsStream = true
    }
  }

  // if they support streaming and allow posting continue
  // otherwise quit
  if ! self.info.supportsStream || ! self.info.allowsPosting {
    if self.debug {
      log.Println(self.info.supportsStream, self.info.allowsPosting)
    }

    self.Quit()
    return
  }
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

  go func() {
    for {
      msg_id := <- self.sync
      for self.reading {
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
      msg := d.store.GetMessage(commands[0])
      if msg == nil {
        log.Println("wut? don't have message", commands[0])
        self.Quit()
        return
      } 
      err = self.SendMessage(msg, d)
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
    } else {
      log.Printf("invalid response from outbound feed: '%d %s'", code, line)
    }
  }
}

// just do it (tm)
func (self *NNTPConnection) SendMessage(message *NNTPMessage, d *NNTPDaemon) error {
  var err error
  self.reading = true
  err = self.txtconn.PrintfLine("TAKETHIS %s", message.MessageID)
  if err != nil {
    log.Println("error in outfeed", err)
    return  err
  }
  wr := self.txtconn.DotWriter()
  err = message.WriteTo(wr, "\r\n")
  wr.Close()
  self.reading = false
  if err != nil {
    log.Printf("failed to send %s via feed: %s", message.MessageID, err)
    return err
  }
  return nil
}

// handle inbound connection
func (self *NNTPConnection) HandleInbound(d *NNTPDaemon) {

  
  // intitiate handshake
  var err error
  self.info.mode = "STREAM"
  log.Println("Incoming nntp connection from", self.conn.RemoteAddr())
  // send welcome
  greet := "2nd generation overchan NNTP Daemon"
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
      if cmd == "TAKETHIS" {
        if len(commands) == 2 {
          article := commands[1]
          if ValidMessageID(article) {
            file := d.store.CreateTempFile(article)
            for {
              var line string
              line, err = self.ReadLine()
              if err != nil {
                log.Println("error reading", article, err)
                break
              }
              if line == "." {
                break
              } else {
                file.Write([]byte(line))
                file.Write([]byte("\n"))
              }
            }
            file.Close()
            // the send was good
            // tell them
            self.txtconn.PrintfLine("239 %s", article)
            log.Println(self.conn.RemoteAddr(), "got article", article)
            // inform daemon
            d.infeed_load <- article
            log.Println("daemon feed article")
          } else {
            self.txtconn.PrintfLine("439 %s", article)
          }
        }
      }
      // check command
      if cmd == "CHECK" {
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
        }
      }
    }
  }
  self.Close()
}

func (self *NNTPConnection) sendCapabilities() {
  wr := self.txtconn.DotWriter()
  io.WriteString(wr, "101 we can haz do things\n")
  io.WriteString(wr, "VERSION 2\n")
  io.WriteString(wr, "IMPLEMENTATION srndv2 better than SRNd\n")
  io.WriteString(wr, "STREAMING\n")
  io.WriteString(wr, "READER\n")
  wr.Close()
}

func (self *NNTPConnection) Quit() {
  if ! self.inbound {
    self.txtconn.PrintfLine("QUIT")
  }
  self.Close()
}

func (self *NNTPConnection) ReadLine() (string, error) {
  line, err := self.txtconn.ReadLine()
  if err != nil {
    log.Println("error reading line in feed", err)
    return "", err
  }
  if self.debug {
    log.Println(self.conn.RemoteAddr(), "recv line", line)
  }
  return line, nil
}

// close the connection
func (self *NNTPConnection) Close() {
  err := self.conn.Close()
  if err != nil {
    log.Println(self.conn.RemoteAddr(), err)
  }
  log.Println(self.conn.RemoteAddr(), "Closed Connection")
}
