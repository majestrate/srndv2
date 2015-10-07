//
// mod.go
// post moderation
//
package srnd

import (
  "bytes"
  "errors"
  "fmt"
  "io"
  "log"
  "net/http"
  "os"
  "strings"
)


// regenerate pages function
type RegenFunc func (newsgroup, msgid, root string, page int)

// does an action for the administrator
// takes in json
type AdminFunc func (param map[string]interface{}) (string, error)

// interface for moderation ui
type ModUI interface {

  // channel for daemon to poll for nntp articles from the mod ui
  MessageChan() chan NNTPMessage

  // check if this key is allowed to access
  // return true if it can otherwise false
  CheckKey(privkey string) (bool, error)

  // serve the base page
  ServeModPage(wr http.ResponseWriter, r *http.Request)
  // handle a login POST request
  HandleLogin(wr http.ResponseWriter, r *http.Request)
  // handle a delete article request 
  HandleDeletePost(wr http.ResponseWriter, r *http.Request)
  // handle a ban address request
  HandleBanAddress(wr http.ResponseWriter, r *http.Request)
  // handle an unban address request
  HandleUnbanAddress(wr http.ResponseWriter, r *http.Request)
  // handle add a pubkey
  HandleAddPubkey(wr http.ResponseWriter, r *http.Request)
  // handle removing a pubkey
  HandleDelPubkey(wr http.ResponseWriter, r *http.Request)
  // handle key generation
  HandleKeyGen(wr http.ResponseWriter, r *http.Request)
  // handle admin command
  HandleAdminCommand(wr http.ResponseWriter, r *http.Request)
}

type ModEvent interface {
  // turn it into a string for putting into an article
  String() string
  // what type of mod event
  Action() string
  // what reason for the event
  Reason() string
  // what is the event acting on
  Target() string
  // scope of the event, regex of newsgroup
  Scope() string
  // when this mod event expires, unix nano
  Expires() int64
}

type simpleModEvent string

func (self simpleModEvent) String() string {
  return string(self)
}

func (self simpleModEvent) Action() string {
  return strings.Split(string(self), " ")[0]
}

func (self simpleModEvent) Reason() string {
  return ""
}

func (self simpleModEvent) Target() string {
  return strings.Split(string(self), " ")[1]
}

func (self simpleModEvent) Scope() string {
  // TODO: hard coded
  return "overchan.*"
}

func (self simpleModEvent) Expires() int64 {
  // no expiration
  return -1
}

// create an overchan-delete mod event
func overchanDelete(msgid string) ModEvent {
  return simpleModEvent(fmt.Sprintf("delete %s", msgid))
}

// create an overchan-inet-ban mod event
func overchanInetBan(encAddr, key string, expire int64) ModEvent {
  return simpleModEvent(fmt.Sprintf("overchan-inet-ban %s:%s:%d", encAddr, key, expire))
}

// moderation message
// wraps multiple mod events
// is turned into an NNTPMessage later
type ModMessage []ModEvent

// write this mod message's body
func (self ModMessage) WriteTo(wr io.Writer, delim []byte) (err error) {
  // write body
  for _, ev := range(self) { 
    _, err = io.WriteString(wr, ev.String())
    _, err = wr.Write(delim)
  }
  return
}



func ParseModEvent(line string) ModEvent {
  return simpleModEvent(line)
}

// wrap mod message in an nntp message
// does not sign
func wrapModMessage(mm ModMessage) NNTPMessage {
  pathname := "nntpchan.censor"
  nntp := nntpArticle{
    headers: make(ArticleHeaders),
  }
  nntp.headers.Set("Newsgroups", "ctl")
  nntp.headers.Set("Content-Type", "text/plain; charset=UTF-8")
  nntp.headers.Set("Message-ID", genMessageID(pathname))
  nntp.headers.Set("Date", timeNowStr())
  nntp.headers.Set("Path", pathname)
  // todo: set these maybe?
  nntp.headers.Set("From", "anon <a@n.on>")
  nntp.headers.Set("Subject", "censor")

  var buff bytes.Buffer
  // crlf delimited
  _ = mm.WriteTo(&buff, []byte{13,10})
  // create plaintext attachment, cut off last 2 bytes
  str := buff.String()
  nntp.message = createPlaintextAttachment(str[:len(str)-2])
  return nntp
}


type ModEngine interface {
  // chan to send the mod engine posts
  // assumes ctl newsgroup only
  MessageChan() chan NNTPMessage

  // delete post of a poster
  DeletePost(msgid string, regen RegenFunc) error
  // ban a cidr
  BanAddress(cidr string) error
  // do we allow this public key to delete?
  AllowDelete(pubkey string) bool
  // do we allow this public key to ban?
  AllowBan(pubkey string) bool
}

type modEngine struct {
  database Database
  store ArticleStore
  chnl chan NNTPMessage
}

func (self modEngine) MessageChan() chan NNTPMessage {
  return self.chnl
}

func (self modEngine) BanAddress(cidr string) (err error) {
  return self.database.BanAddr(cidr)
}

func (self modEngine) DeletePost(msgid string, regen RegenFunc) (err error) {
  hdr := self.store.GetHeaders(msgid)
  if hdr == nil {
    return errors.New("no such message on filesystem: "+msgid)
  } else {
    ref := hdr.Get("References", "")
    group := hdr.Get("Newsgroups", "")
    var delposts []string
    var page int64
    if ref == "" {
      // is root post
      // delete replies too
      repls := self.database.GetThreadReplies(msgid, 0)
      if repls == nil {
        log.Println("cannot get thread replies for", msgid)
      } else {
        delposts = append(delposts, repls...)
      }
      
      _, page, err = self.database.GetPageForRootMessage(msgid)
      // delete thread presence
      self.database.DeleteThread(msgid)
      ref = msgid
    } else {
      _, page, err = self.database.GetPageForRootMessage(ref) 
    }
    delposts = append(delposts, msgid)
    // get list of files to delete
    var delfiles []string
    for _, delmsg := range delposts {
      article := self.store.GetFilename(delmsg)
      delfiles = append(delfiles, article)
      // get attachments for post
      atts := self.database.GetPostAttachments(delmsg)
      if atts != nil {
        for _, att := range(atts) {
          img := self.store.AttachmentFilepath(att)
          thm := self.store.ThumbnailFilepath(att)
          delfiles = append(delfiles, img, thm)
        }
      }
      // delete article from post database
      self.database.DeleteArticle(delmsg)
      // ban article
      self.database.BanArticle(delmsg, "deleted by moderator")
    }
    // delete all files
    for _, f := range delfiles {
      log.Printf("delete file: %s", f)
      os.Remove(f)
    }
    if regen != nil {
      regen(group, msgid, ref, int(page))
    }
  }
  return nil
}

// TODO: permissions
func (self modEngine) AllowBan(pubkey string) bool {
  return self.database.CheckModPubkeyGlobal(pubkey)
}
// TODO: permissions
func (self modEngine) AllowDelete(pubkey string) bool {
  return self.database.CheckModPubkeyGlobal(pubkey)
}

// run a mod engine logic mainloop
func RunModEngine(mod ModEngine, regen RegenFunc) {
  
  chnl := mod.MessageChan()
  for {
    nntp := <- chnl
    // sanity check
    if nntp.Newsgroup() == "ctl" {
      inner_nntp := nntp.Signed()
      if inner_nntp != nil {
        // okay this message should be good
        pubkey := nntp.Pubkey()
        for _, line := range strings.Split(inner_nntp.Message(), "\n") {
          line = strings.Trim(line, "\r\t\n")
          ev := ParseModEvent(line)
          action := ev.Action()
          if action == "delete" {
            msgid := ev.Target()
            // this is a delete action
            if mod.AllowDelete(pubkey) {
              err := mod.DeletePost(msgid, regen)
              if err != nil {
                log.Println(msgid, err)
              }
            } else {
              log.Printf("pubkey=%s will not delete %s not trusted", pubkey, msgid)
            }
          } else if action == "overchan-inet-ban" {
            // ban action
            target := ev.Target()
            parts := strings.Split(target, ":")
            if len(parts) == 3 {
              encaddr, key := parts[0], parts[1]
              cidr := decAddr(encaddr, key)
              if cidr == "" {
                log.Println("failed to decrypt inet ban")
              } else if mod.AllowBan(pubkey) {
                err := mod.BanAddress(cidr)
                if err != nil {
                  log.Println(cidr, err)
                }
              }
            } else {
              log.Printf("invalid overchan-inet-ban: target=%s", target)
            }
          }
        }
      }
    }
  }  
}
