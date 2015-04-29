package nacl


// #cgo LDFLAGS: -lsodium -Lbuild.dir/src/libsodium
// #cgo CFLAGS: -Ideps/libsodium/src/libsodium/include
// #include "sodium.h"
import "C"



func RandBytes(size int) *Buffer {
  if size > 0 {
    buff := Malloc(size)
    C.randombytes_buf(buff.ptr, buff.size)
    return buff
  }
  return nil
}
