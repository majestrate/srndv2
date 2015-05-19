package nacl



// #cgo pkg-config: sodium
//
import "C"

import (
  "log"
)



func testSign(bufflen int, keys *KeyPair) {
  log.Printf("Test %dKB sign/verify...", bufflen)
  msg := RandBytes(bufflen)
  sig := CryptoSign(msg.Data(), keys.sk.Data())
  if ! CryptoVerify(msg.Data(), sig, keys.pk.Data()) {
    log.Fatal("Failed")
  }
  msg.Free()
}

// test all crypto functions
func TestAll() {
  log.Println("Begin Crypto test")
  
  bufflen := 32

  b := RandBytes(bufflen)
  defer b.Free()
  if b.Length() != bufflen {
    log.Fatal("nacl.RandBytes() failed length test")
  }
  keys := GenKeypair()
  defer keys.Free()
  for n := 1 ; n < 16 ; n++ {
    testSign(n * 1024, keys)
  }
  log.Println("Crypto Test Done")
}


// initialize sodium
func init() {
  status := C.sodium_init()
  if status == -1 {
    log.Fatal("failed to initialize libsodium, status", status)
  }
  version_ptr := C.sodium_version_string()
  
  log.Println("Intialized Sodium", C.GoString(version_ptr))
  TestAll()
}
