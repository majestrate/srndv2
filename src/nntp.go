//
// nntp.go
//
package main

import (
  "bufio"
  "bytes"
  "io/ioutil"
  "log"
  "net"
  "strings"
)
  
type ConnectionInfo struct {
  mode string
  newsgroup string
  allowsPosting bool 
  supportsStream bool 
}

type NNTPConnection struct {
  conn net.Conn
  reader *bufio.Reader
  inbound bool
  debug bool
  info *ConnectionInfo
  policy *FeedPolicy
  send chan *NNTPMessage
}

func (self *NNTPConnection) HandleOutbound(d *NNTPDaemon) {
  var err error
  line := self.ReadLine()
  self.info.allowsPosting = strings.HasPrefix(line, "200 ")
  // they allow posting
  // send capabilities command
  err = self.SendLine("CAPABILITIES")
  
  // get capabilites
  for {
    line = strings.ToLower(self.ReadLine())
    if line == ".\r\n" {
      // done reading capabilities
      break
    }
    if line == "streaming\r\n" {
      self.info.supportsStream = true
    } else if line == "postihavestreaming\r\n" {
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
  err = self.SendLine("MODE STREAM")
  if err != nil {
    return 	
  }
  line = self.ReadLine()
  if strings.HasPrefix(line, "203 ") {
    self.info.mode = "stream"
    log.Println("streaming mode activated")
  } else {
    self.Quit()
    return
  }
  // mainloop
  for  {
    if err != nil {
      // error from previous
      break
    }
    // poll for new message
    message := <- self.send
    err = self.SendMessage(message, d)
    if err != nil {
      log.Println(err)
      break
    }
  }
}

func (self *NNTPConnection) SendMessage(message *NNTPMessage, d *NNTPDaemon) error {
  var err error
  var line string
  // check if we allow it
  if self.policy == nil {
    // we have no policy so reject
    return nil
  }
  if ! self.policy.AllowsNewsgroup(message.Newsgroup) {
    log.Println("not federating article", message.MessageID, "beause it's in", message.Newsgroup)
    return nil
  }
  if ! self.policy.FederateNewsgroup(message.Newsgroup) {
    log.Println("dont federate article", message.MessageID, "disallowed by feed policy")
    return nil
  }
  // send check
  err = self.SendLine("CHECK "+message.MessageID)
  line = self.ReadLine()
  if strings.HasPrefix(line, "238 ") {
    // accepted
    // send it
    err = self.SendLine("TAKETHIS "+message.MessageID)
    if err != nil {
      log.Println("error in outfeed", err)
      return  err
    }
    // load file
    data, err := ioutil.ReadFile(d.store.GetFilename(message.MessageID))
    if err != nil {
      log.Fatal("failed to read article", message.MessageID)
      self.Quit()
      return err
    }
    // split into lines
    parts := bytes.Split(data,[]byte{'\n'})
    // for each line send it
    for idx := range parts {
      ba := parts[idx]
      err = self.SendBytes(ba)
      err = self.Send("\r\n")
    }
    // send delimiter
    err = self.SendLine(".")
    if err != nil {
      log.Println("failed to send")
      self.Quit()
      return err
    }
    // check for success / fail
    line := self.ReadLine()
    if strings.HasPrefix(line, "239 ") {
      log.Println("Article", message.MessageID, "sent")
    } else {
      log.Println("Article", message.MessageID, "failed to send", line)
    }
    // done
    return nil
  } else if strings.HasPrefix(line, "435 ") {
    // already have it
    if self.debug {
      log.Println(message.MessageID, "already owned by remote")
    }
  } else if strings.HasPrefix(line, "437 ") {
    // article banned
    log.Println(message.MessageID, "was banned")
  }
  if err != nil {
    self.Quit()
    log.Println("failure in outfeed", err)	
    return err
  }
  return nil
}

// handle inbound connection
func (self *NNTPConnection) HandleInbound(d *NNTPDaemon) {
  var err error
  self.info.mode = "STREAM"
  log.Println("Incoming nntp connection from", self.conn.RemoteAddr())
  // send welcome
  self.SendLine("200 ayy lmao we are SRNd2, posting allowed")
  for {
    if err != nil {
      log.Println("failure in infeed", err)
      self.Quit()
      return
    }
    // read line and break if needed
    line := self.ReadLine()
    if len(line) == 0 {
      break
    }
    
    // parse line
    _line := strings.Replace(line, "\n", "", -1)
    _line = strings.Replace(_line, "\r", "", -1)
    commands := strings.Split(_line, " ")
    cmd := strings.ToUpper(commands[0])

    // capabilities command
    if cmd == "CAPABILITIES" {
      self.sendCapabilities()
    } else if cmd == "MODE" { // mode switch
      if len(commands) == 2 {
        // get mode
        mode := strings.ToUpper(commands[1])
        // mode reader not implemented
        if mode == "READER" {
          self.SendLine("501 no reader mode")
        } else if mode == "STREAM" {
          // mode stream is implemented
          self.info.mode = mode
          self.SendLine("203 stream as desired")
        } else {
          // other modes not implemented
          self.SendLine("501 unknown mode")
        }
      } else {
        self.SendLine("500 syntax error")
      }
    } else if self.info.mode == "STREAM" { // we are in stream mode
      if cmd == "TAKETHIS" {
        if len(commands) == 2 {
          article := commands[1]
          if ValidMessageID(article) {
            file := d.store.CreateFile(article)
            var rewrote_path bool
            for {
              line := self.ReadLine()
              // unexpected close
              if len(line) == 0 {
                log.Fatal(self.conn.RemoteAddr(), "unexpectedly closed connection")
              }
              // rewrite path header
              // add us to the path
              if ! rewrote_path && strings.HasPrefix(line, "Path: ") {
                line = "Path: " + d.instance_name + "!" + line[6:]
              }
              // done reading
              if line == ".\r\n" {
                break
              } else {
                line = strings.Replace(line, "\r\n", "\n", -1)
                file.Write([]byte(line))
              }
            }
            file.Close()
            // the send was good
            // tell them
            self.SendLine("239 "+article)
            log.Println(self.conn.RemoteAddr(), "got article", article)

            // inform daemon
            d.infeed <- article
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
            self.SendLine("501 bad message id")
            continue
          }
          // do we already have this article?
          if d.store.HasArticle(article) {
            // ya, we got it already
            // tell them to not send it
            self.SendLine("435 "+article+" we have this article")
          } else {
            // nope, we do not have it
            // tell them to send it
            self.SendLine("238 "+article+" we want this article please give it")
          }
        }
      }
    }
  }
  self.Close()
}

func (self *NNTPConnection) sendCapabilities() {
  self.SendLine("101 we can do stuff")
  self.SendLine("VERSION 2")
  self.SendLine("IMPLEMENTATION srndv2 better than SRNd")
  self.SendLine("STREAMING")
  self.SendLine(".")
}

func (self *NNTPConnection) Quit() {
  if ! self.inbound {
    self.SendLine("QUIT")
    _ = self.ReadLine()
  }
  self.Close()
}

func (self *NNTPConnection) ReadLine() string {
  line, err := self.reader.ReadString('\n')
  if err != nil {
    return ""
  }
  //line = strings.Replace(line, "\n", "", -1)
  //line = strings.Replace(line, "\r", "", -1)
  if self.debug {
    log.Println(self.conn.RemoteAddr(), "recv line", line)
  }
  return line
}

// send a line
func (self *NNTPConnection) SendLine(line string) error {
  if self.debug {
    log.Println(self.conn.RemoteAddr(), "send line", line)
  }
  return self.Send(line+"\r\n")
}

// send data
func (self *NNTPConnection) Send(data string) error {
  _, err := self.conn.Write([]byte(data))
  return err
}

// send data
func (self *NNTPConnection) SendBytes(data []byte) error {
  _ , err := self.conn.Write(data)
  return err
}

// close the connection
func (self *NNTPConnection) Close() {
  err := self.conn.Close()
  if err != nil {
    log.Println(self.conn.RemoteAddr(), err)
  }
  log.Println(self.conn.RemoteAddr(), "Closed Connection")
}
