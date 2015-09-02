//
// nntp.go -- nntp interface for peering
//
package srnd

import (
  "bufio"
  "io"
  "log"
  "net/textproto"
  "os"
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
func (self nntpConnection) modeSwitch(mode string, conn *textproto.Conn) (success bool, err error) {
  self.access.Lock()
  mode = strings.ToUpper(mode)
  conn.PrintfLine("MODE %s", mode)
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
  var code int
  var line string
  for err == nil {
    code, line, err = conn.ReadCodeLine(-1)
    if err == nil {
      if code == 200 {
        // send capabilities
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

// run the mainloop for this connection
// stream if true means they support streaming mode
// reader if true means they support reader mode
func (self nntpConnection) runConnection(daemon NNTPDaemon, inbound, stream, reader bool, conn *textproto.Conn) {

  var err error
  var code int
  var line string
  var success bool

  for err == nil {
    if self.mode == "" {
      if inbound  {
        // no mode set and inbound
        line, err = conn.ReadLine()
      } else {
        // set out mode we are outbound
        if stream {
          success, err = self.modeSwitch("STREAM", conn)
        } else if reader {
          success, err = self.modeSwitch("READER", conn)
        }
        if success {
          log.Println("mode set to", self.mode)
        } else {
          // bullshit
          // we can't do anything so we quit
          log.Println(self.name, "can't stream or read, wtf?")
          conn.PrintfLine("QUIT")
          conn.Close()
          return
        }
      }
    } else if self.mode == "STREAM" {
      // we're in streaming mode
      // let's pipeline this shit
      select {
        // we've are going to ask for an article in READER mode
      case msgid := <- self.article:
        if reader && ValidMessageID(msgid) {
          // switch to reader mode
          success, err = self.modeSwitch("READER", conn)
          if success {
            // send the request
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
            // now switch back to streaming mode
            log.Println(self.name, "switch back to streaming mode")
            success, err = self.modeSwitch("STREAM", conn)
            if ! success {
              log.Println(self.name, "failed to switch back to streaming mode? wtf!")
            }
          } else {
            log.Println(self.name, "failed to set reader mode")
          }
        }
      case msgid := <- self.check:
        conn.PrintfLine("CHECK %s", msgid)
      case msgid := <- self.take:
        // send a file via TAKETHIS
        if ValidMessageID(msgid) {
          fname := daemon.store.GetFilename(line)
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
      default:
        code, line, err = conn.ReadCodeLine(-1)
        if code == 238 {
          if ValidMessageID(line) {
            log.Println("sending", line, "to", self.name)
            // send the article to us
            self.take <- line
          }
        } else if code == 239 {
          // successful TAKETHIS
          log.Println(line, "sent via", self.name)
          // TODO: remember success 
        } else if code == 431 {
          // CHECK said we would like this article later
          log.Println("defer sending", line, "to", self.name)
          go self.articleDefer(line)
        } else if code == 439 {
          // TAKETHIS failed
          log.Println(line, "was not sent to", self.name, "denied")
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
              hdr, err = conn.ReadMIMEHeader()
              // check the headers and see if we want this article
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
                code = 439
              } else if ! ( ValidMessageID(msgid) && ( reference != "" && ValidMessageID(reference) ) ) {
                // invalid message id or reference
                code = 439
              } else if daemon.store.HasArticle(msgid) {
                // we already have this article locally
                code = 439
              } else if daemon.database.HasArticle(msgid) {
                // we have already seen this article
                log.Println(self.name, "already seen", msgid)
                code = 439
              } else if reference != "" && daemon.database.IsExpired(reference) {
                // this belongs to a root post that is expired or banned
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
                      code = 439
                    }
                  } else {
                    // we allow anon posts that are plain
                    code = 239
                  }
                } else {
                  // we don't allow anon posts of any kind
                  code = 439
                }
              } else {
                // check for banned address
                banned, err := daemon.database.CheckEncIPBanned(encaddr)
                if err == nil {
                  if banned {
                    // this address is banned
                    code = 439
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
              }
            }
          }
        }
      }
    } else if self.mode == "READER" {
      
    }
  }
  log.Println("run connection got error", err)
  conn.Close()
}

func (self nntpConnection) articleDefer(msgid string) {
  time.Sleep(time.Second * 90)
  self.check <- msgid
}
