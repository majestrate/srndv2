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
  "fmt"
  "io"
  "log"
  "net/http"
  //"strings"
)

type httpModUI struct {
  modMessageChan chan *NNTPMessage
  database Database
  store *sessions.CookieStore
  prefix string
  mod_prefix string
}

func createHttpModUI(frontend httpFrontend) httpModUI {
  return httpModUI{make(chan *NNTPMessage), frontend.daemon.database, frontend.store, frontend.prefix, frontend.prefix + "mod/"}
}

func (self httpModUI) CheckKey(privkey string) (bool, error) {
  privkey_bytes, err := hex.DecodeString(privkey)
  if err == nil {
    pubkey_bytes := nacl.GetSignPubkey(privkey_bytes)
    if pubkey_bytes != nil {
      pubkey := hex.EncodeToString(pubkey_bytes)
      return self.database.CheckModPubkey(pubkey), nil
    }
  }
  log.Println("invalid key format for key", privkey)
  return false, err
}


func (self httpModUI) MessageChan() chan *NNTPMessage {
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
