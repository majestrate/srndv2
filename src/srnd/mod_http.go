//
// mod_http.go
//
// http mod panel
//

package srnd

import (
  "github.com/majestrate/srndv2/src/nacl"
  "github.com/gorilla/sessions"
  "encoding/hex"
  "encoding/json"
  "fmt"
  "io"
  "log"
  "net/http"
  "os"
  "strings"
)

type httpModUI struct {
  regen func (ArticleEntry)
  delete func (string)
  modMessageChan chan NNTPMessage
  database Database
  articles ArticleStore
  store *sessions.CookieStore
  prefix string
  mod_prefix string
}

func createHttpModUI(frontend httpFrontend) httpModUI {
  return httpModUI{frontend.Regen, frontend.deleteThreadMarkup, make(chan NNTPMessage), frontend.daemon.database, frontend.daemon.store, frontend.store, frontend.prefix, frontend.prefix + "mod/"}
}

// TODO: check for different levels of permissions
func (self httpModUI) CheckKey(privkey string) (bool, error) {
  privkey_bytes, err := hex.DecodeString(privkey)
  if err == nil {
    pubkey_bytes := nacl.GetSignPubkey(privkey_bytes)
    if pubkey_bytes != nil {
      pubkey := hex.EncodeToString(pubkey_bytes)
      if self.database.CheckModPubkey(pubkey) {
        return true, nil
      } else if self.database.CheckModPubkeyGlobal(pubkey) {
        return true, nil
      } else {
        return false, nil
      }
    }
  }
  log.Println("invalid key format for key", privkey)
  return false, err
}


func (self httpModUI) MessageChan() chan NNTPMessage {
  return self.modMessageChan
}

func (self httpModUI) getSession(r *http.Request) *sessions.Session {
  s, _ := self.store.Get(r, "nntpchan-mod")
  return s
}

// returns true if the session is okay
// otherwise redirect to login page
func (self httpModUI) checkSession(r *http.Request) bool {
  s := self.getSession(r)
  k, ok := s.Values["privkey"]
  if ok {
    ok, err := self.CheckKey(k.(string))
    if err != nil {
      return false
    }
    return ok
  }
  return false
}

func (self httpModUI) writeTemplate(wr http.ResponseWriter, name string) {
  self.writeTemplateParam(wr, name, nil)
}

func (self httpModUI) writeTemplateParam(wr http.ResponseWriter, name string, param map[string]string) {
  if param == nil {
    param = make(map[string]string)
  }
  param["prefix"] = self.prefix
  param["mod_prefix"] = self.mod_prefix
  io.WriteString(wr, renderTemplate(name, param))  
}

func (self httpModUI) HandleAddPubkey(wr http.ResponseWriter, r *http.Request) {
}
func (self httpModUI) HandleDelPubkey(wr http.ResponseWriter, r *http.Request) {
}
func (self httpModUI) HandleBanAddress(wr http.ResponseWriter, r *http.Request) {
}
func (self httpModUI) HandleDeletePost(wr http.ResponseWriter, r *http.Request) {
  if self.checkSession(r) {
    resp := make(map[string]interface{})
    path := r.URL.Path
    if strings.Count(path, "/") > 2 {
      // get the long hash from the request path
      // TODO: prefix detection
      longhash := strings.Split(path, "/")[3]
      // get the MessageID given the long hash
      msg, err := self.database.GetMessageIDByHash(longhash)
      if err == nil {
        msgid := msg.MessageID()
        delmsgs := []string{}
        // get headers
        hdr := self.articles.GetHeaders(msgid)
        if hdr == nil {
          resp["error"] = fmt.Sprintf("message %s is not on the filesystem? wtf!", msgid)
        } else {
          ref := hdr.Get("Reference", "")
          if ref == "" {
            // load replies
            replies := self.database.GetThreadReplies(ref, 0)
            if replies != nil {
              delmsgs = append(delmsgs, replies...)
            }
            // pre-emptively delete thread html page
            self.delete(msgid)
          }
          delmsgs = append(delmsgs, msgid)
          deleted := []string{}
          report := []string{}
          // now delete them all
          for _, delmsgid := range delmsgs {
            err = self.deletePost(delmsgid)
            if err == nil {
              deleted = append(deleted, delmsgid)
            } else {
              report = append(report, delmsgid)
              log.Printf("error when removing %s, %s", delmsgid, err)
            }
          }
          resp["deleted"] = deleted
          resp["notdeleted"] = report
          // only regen threads when we delete a non root port
          if ref != "" {
            group := hdr.Get("Newsgroups", "")
            self.regen(ArticleEntry{
              ref, group,
            })
          }
        }
      } else {
        if msg.MessageID() == "" {
          resp["error"] = "no such message with that hash"
        } else {
          resp["error"] = err.Error()
        }
      }
      // send response
      enc := json.NewEncoder(wr)
      enc.Encode(resp)
    }
  } else {
    wr.WriteHeader(403)
  }
}

func (self httpModUI) deletePost(msgid string) (err error) {
  msgfilepath := self.articles.GetFilename(msgid)
  delfiles := []string{msgfilepath}
  atts := self.database.GetPostAttachments(msgid)
  if atts != nil {
    for _, att := range atts {
      img := self.articles.AttachmentFilepath(att)
      thm := self.articles.ThumbnailFilepath(att)
      delfiles = append(delfiles, img, thm)
    }
  }
  for _ , fpath := range delfiles {
    log.Printf("remove file %s", fpath)
    os.Remove(fpath)
  }
  return self.database.DeleteArticle(msgid)
}

func (self httpModUI) HandleLogin(wr http.ResponseWriter, r *http.Request) {
  privkey := r.FormValue("privkey")
  msg := "failed login: "
  if len(privkey) == 0 {
    msg += "no key"
  } else {
    ok, err := self.CheckKey(privkey)
    if err != nil {
      msg += fmt.Sprintf("%s", err)
    } else if ok {
      msg = "login okay"
      sess := self.getSession(r)
      sess.Values["privkey"] = privkey
      sess.Save(r, wr)
    } else {
      msg += "invalid key"
    }
  }
  self.writeTemplateParam(wr, "modlogin_result.mustache", map[string]string { "message" : msg })
}

func (self httpModUI) HandleKeyGen(wr http.ResponseWriter, r *http.Request) {
  pk, sk := newSignKeypair()
  tripcode := makeTripcode(pk)
  self.writeTemplateParam(wr, "keygen.mustache", map[string]string {"public" : pk, "secret" : sk, "tripcode" : tripcode})
}

func (self httpModUI) ServeModPage(wr http.ResponseWriter, r *http.Request) {
  if self.checkSession(r) {
    // we are logged in
    // serve mod page
    self.writeTemplate(wr, "modpage.mustache")
  } else {
    // we are not logged in
    // serve login page
    self.writeTemplate(wr, "modlogin.mustache")
  }

}
