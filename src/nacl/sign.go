
package nacl



// #cgo LDFLAGS: -lsodium -Lbuild.dir/prefix/lib
// #cgo CFLAGS: -Ibuild.dir/prefix/include
// #include "sodium.h"
import "C"

import (
  "log"
)

// sign data with secret key sk
func CryptoSign(msg, sk []byte) []byte {
  msgbuff := NewBuffer(msg)
  defer msgbuff.Free()
  skbuff := NewBuffer(sk)
  defer skbuff.Free()
  if skbuff.size != C.crypto_sign_bytes() {
    log.Println("nacl.CryptoSign() invalid secret key size", len(sk))
    return nil
  }
  
  // allocate the signature buffer
  sig := malloc(C.crypto_sign_bytes())
  defer sig.Free()
  // compute signature
  siglen := C.ulonglong(0)
  res := C.crypto_sign_detached(sig.uchar(), &siglen, msgbuff.uchar(), C.ulonglong(msgbuff.size), skbuff.uchar())
  if res == 0 && siglen == C.ulonglong(C.crypto_sign_bytes()) {
    // return copy of signature buffer
    return sig.Bytes()
  }
  // failure to sign
  log.Println("nacl.CryptoSign() failed")
  return nil
}


// verfiy a detached signature
// return true on valid otherwise false
func CryptoVerify(msg, sig, pk []byte) bool {
  msg_buff := NewBuffer(msg)
  defer msg_buff.Free()
  sig_buff := NewBuffer(sig)
  defer sig_buff.Free()
  pk_buff := NewBuffer(pk)
  defer pk_buff.Free()

  if pk_buff.size != C.crypto_sign_publickeybytes() {
    log.Println("nacl.CryptoVerify() invalid public key size", len(pk))
    return false
  }
  
  // invalid sig size
  if sig_buff.size != C.crypto_sign_bytes() {
    log.Println("nacl.CryptoVerify() invalid signature length", len(sig))
    return false
  }
  return C.crypto_sign_verify_detached(sig_buff.uchar(), msg_buff.uchar(), C.ulonglong(msg_buff.length), pk_buff.uchar()) == 0
}
