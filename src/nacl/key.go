
package nacl


// #cgo LDFLAGS: -lsodium -Lbuild.dir/src/libsodium
// #cgo CFLAGS: -Ideps/libsodium/src/libsodium/include
// #include "sodium.h"
import "C"

import (
  "encoding/hex"
  "fmt"
  "log"
)

type KeyPair struct {
  pk *Buffer
  sk *Buffer
}

// free this keypair from memory
func (self *KeyPair) Free() {
  self.pk.Free()
  self.sk.Free()
}

func (self *KeyPair) Secret() []byte {
  return self.sk.Bytes()
}

func (self *KeyPair) Public() []byte {
  return self.pk.Bytes()
}

// generate a keypair
func GenKeypair() *KeyPair {
  sk_len := C.crypto_sign_secretkeybytes()
  sk := malloc(sk_len)
  pk_len := C.crypto_sign_publickeybytes()
  pk := malloc(pk_len)
  res := C.crypto_sign_keypair(pk.uchar(), sk.uchar())
  if res == 0 {
    return &KeyPair{pk,sk}
  }
  log.Println("nacl.GenKeypair() failed to generate keypair")
  pk.Free()
  sk.Free()
  return nil
}

/*
func LoadKeypair(sk []byte) *KeyPair {
  sk_len := C.crypto_sign_secretkeybytes()
  if C.size_t(len(sk)) != sk_len {
    log.Println("nacl.LoadKeypair() invalid secret key size", len(sk))
    return nil
  }
  skbuff := NewBuffer(sk)
  pk_len := C.crypto_sign_publickeybytes()
  pkbuff := malloc(pk_len)
  C.crypto_sign_sk_to_pk(pkbuff.uchar(), skbuff.uchar())
  return &KeyPair{pkbuff, skbuff}
} */

func (self *KeyPair) String() string {
  return fmt.Sprintf("pk=%s sk=%s", hex.EncodeToString(self.pk.Data()), hex.EncodeToString(self.sk.Data()))
}
