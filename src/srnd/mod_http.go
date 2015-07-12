//
// mod_http.go
//
// http mod panel
//

package srnd

import (
  "github.com/majestrate/srndv2/src/nacl"
  "encoding/hex"
  "log"
)

type httpModUI struct {
  modMessageChan chan *NNTPMessage
  database Database
}

func createHttpModUI(daemon *NNTPDaemon) httpModUI {
  return httpModUI{make(chan *NNTPMessage), daemon.database}
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
