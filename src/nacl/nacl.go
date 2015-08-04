package nacl

// #include <sodium.h>
// #cgo pkg-config: libsodium
import "C"

import (
  "bytes"
  "log"
  "os"
)

// return how many bytes overhead does CryptoBox have
func CryptoBoxOverhead() int {
  return int(C.crypto_box_macbytes())
}

// size of crypto_box public keys
func CryptoBoxPubKeySize() int {
  return int(C.crypto_box_publickeybytes())
}

// size of crypto_box private keys
func CryptoBoxPrivKeySize() int {
  return int(C.crypto_box_secretkeybytes())
}

// size of crypto_sign public keys
func CryptoSignPubKeySize() int {
  return int(C.crypto_sign_publickeybytes())
}

// size of crypto_sign private keys
func CryptoSignPrivKeySize() int {
  return int(C.crypto_sign_secretkeybytes())
}

func testSign(bufflen int, keys *KeyPair) {
  log.Printf("Test %d sign/verify...", bufflen)
  msg := RandBytes(bufflen)
  sig := CryptoSignDetached(msg, keys.sk.Data())
  if ! CryptoVerifyDetached(msg, sig, keys.pk.Data()) {
    log.Fatal("Failed")
  }
}

func testFucky(bufflen int, keys *KeyPair) {
  log.Printf("Test %d fucky sign/verify...", bufflen)
  msg := RandBytes(bufflen)
  sig := CryptoSignFucky(msg, keys.sk.Data())
  if ! CryptoVerifyFucky(msg, sig, keys.pk.Data()) {
    log.Fatal("Failed")
  }
}

func testBox(bufflen int, tokey, fromkey *KeyPair) {
  log.Printf("Test %d box/box_open...", bufflen)
  msg := RandBytes(bufflen)
  nounce := NewBoxNounce()
  box := CryptoBox(msg, nounce, tokey.Public(), fromkey.Secret())
  if box == nil {
    log.Fatal("CryptoBox() failed")
  }
  msg_open := CryptoBoxOpen(box, nounce, tokey.Secret(), fromkey.Public())
  if ! bytes.Equal(msg, msg_open) {
    log.Fatalf("CryptoBoxOpen() failed: %d vs %d", len(msg), len(msg_open))
  }
}

// test all crypto functions
func TestAll() {
  log.Println("Begin Crypto test")
  
  bufflen := 128

  b := RandBytes(bufflen)
  if len(b) != bufflen {
    log.Fatal("nacl.RandBytes() failed length test")
  }
  
  for n := 1 ; n < 16 ; n++ {
    key := GenSignKeypair()
    defer key.Free()
    testSign(n * 1024, key)
  }
  
  for n := 1 ; n < 16 ; n++ {
    key := GenSignKeypair()
    defer key.Free()
    testFucky(n * 1024, key)
  }
  
  for n := 1 ; n < 16 ; n++ {
    tokey := GenBoxKeypair()
    fromkey := GenBoxKeypair()
    defer tokey.Free()
    defer fromkey.Free()
    testBox(n * 1024, tokey, fromkey)
  }
  
  
  log.Println("Crypto Test Done")
}


// initialize sodium
func init() {
  status := C.sodium_init()
  if status == -1 {
    log.Fatalf("failed to initialize libsodium status=%d", status)
  }

  if os.Getenv("SODIUM_TEST") == "1" {
    version_ptr := C.sodium_version_string()
    log.Println("Intialized Sodium", C.GoString(version_ptr))
    TestAll()
  }
}
