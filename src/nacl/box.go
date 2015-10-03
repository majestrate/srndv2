package nacl

// #include <sodium.h>
// #cgo LDFLAGS: -lsodium
import "C"

import (
  "log"
)

// encrypts a message to a user given their public key is known
// returns an encrypted box
func CryptoBox(msg, nounce, pk, sk []byte) []byte {
  msgbuff := NewBuffer(msg)
  defer msgbuff.Free()

  // check sizes
  if len(pk) != int(C.crypto_box_publickeybytes()) {
    log.Println("len(pk) != crypto_box_publickey_bytes")
    return nil
  }
  if len(sk) != int(C.crypto_box_secretkeybytes()) {
    log.Println("len(sk) != crypto_box_secretkey_bytes")
    return nil
  }
  if len(nounce) != int(C.crypto_box_macbytes()) {
    log.Println("len(nounce) != crypto_box_macbytes()")
    return nil
  }
  
  pkbuff := NewBuffer(pk)
  defer pkbuff.Free()
  skbuff := NewBuffer(sk)
  defer skbuff.Free()
  nouncebuff := NewBuffer(nounce)
  defer nouncebuff.Free()
  
  resultbuff := malloc(msgbuff.size + nouncebuff.size)
  defer resultbuff.Free()
  res := C.crypto_box_easy(resultbuff.uchar(), msgbuff.uchar(), C.ulonglong(msgbuff.size), nouncebuff.uchar(), pkbuff.uchar(), skbuff.uchar())
  if res != 0 {
    log.Println("crypto_box_easy failed:", res)
    return nil
  }
  return resultbuff.Bytes()
}

// open an encrypted box
func CryptoBoxOpen(box, nounce, sk, pk []byte) []byte {
  boxbuff := NewBuffer(box)
  defer boxbuff.Free()

  // check sizes
  if len(pk) != int(C.crypto_box_publickeybytes()) {
    log.Println("len(pk) != crypto_box_publickey_bytes")
    return nil
  }
  if len(sk) != int(C.crypto_box_secretkeybytes()) {
    log.Println("len(sk) != crypto_box_secretkey_bytes")
    return nil
  }
  if len(nounce) != int(C.crypto_box_macbytes()) {
    log.Println("len(nounce) != crypto_box_macbytes()")
    return nil
  }
    
  pkbuff := NewBuffer(pk)
  defer pkbuff.Free()
  skbuff := NewBuffer(sk)
  defer skbuff.Free()
  nouncebuff := NewBuffer(nounce)
  defer nouncebuff.Free()
  resultbuff := malloc(boxbuff.size - nouncebuff.size)
  defer resultbuff.Free()
  
  // decrypt
  res := C.crypto_box_open_easy(resultbuff.uchar(), boxbuff.uchar(), C.ulonglong(boxbuff.size), nouncebuff.uchar(), pkbuff.uchar(), skbuff.uchar())
  if res != 0 {
    log.Println("crypto_box_open_easy() failed:", res)
    return nil
  }
  // return result
  return resultbuff.Bytes()
}

// generate a new nounce
func NewBoxNounce() []byte {
  return RandBytes(int(C.crypto_box_macbytes()))
}
