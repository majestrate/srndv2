
package nacl

// #cgo pkg-config: libsodium
// #include <sodium.h>
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

// get public key from secret key
func GetPubkey(sk []byte) []byte {
  sk_len := C.crypto_sign_secretkeybytes()
  if C.size_t(len(sk)) != sk_len {
    log.Printf("nacl.GetPubkey() invalid secret key size %d != %d", len(sk), sk_len)
    return nil
  }
  
  pk_len := C.crypto_sign_publickeybytes()
  pkbuff := malloc(pk_len)
  defer pkbuff.Free()

  skbuff := NewBuffer(sk)
  defer skbuff.Free()
  
  res := C.crypto_sign_ed25519_sk_to_pk(pkbuff.uchar(), skbuff.uchar())

  if res != 0 {
    log.Printf("nacl.GetPubkey() failed to get public key from secret key: %d", res)
    return nil
  }
  
  return pkbuff.Bytes()
}

// load keypair from secret key
func LoadKey(sk []byte) *KeyPair {
  pk := GetPubkey(sk)
  if pk == nil {
    log.Println("nacl.LoadKey() failed to load keypair")
    return nil
  }
  pkbuff := NewBuffer(pk)
  skbuff := NewBuffer(sk)
  return &KeyPair{pkbuff, skbuff}
}

// make keypair from seed
func SeedKey(seed []byte) *KeyPair {
  seed_len := C.crypto_sign_seedbytes()
  if C.size_t(len(seed)) != seed_len {
    log.Println("nacl.SeedKey() invalid seed size", len(seed))
    return nil
  }
  seedbuff := NewBuffer(seed)
  defer seedbuff.Free()
  pk_len := C.crypto_sign_publickeybytes()
  sk_len := C.crypto_sign_secretkeybytes()
  pkbuff := malloc(pk_len)
  skbuff := malloc(sk_len)
  res := C.crypto_sign_seed_keypair(pkbuff.uchar(), skbuff.uchar(), seedbuff.uchar())
  if res != 0 {
    log.Println("nacl.LoadKey cannot derive keys from seed", res)
    return nil
  }
  return &KeyPair{pkbuff, skbuff}
}

func (self *KeyPair) String() string {
  return fmt.Sprintf("pk=%s sk=%s", hex.EncodeToString(self.pk.Data()), hex.EncodeToString(self.sk.Data()))
}
