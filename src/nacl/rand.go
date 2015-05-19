package nacl

// #cgo pkg-config: libsodium
import "C"



func RandBytes(size int) *Buffer {
  if size > 0 {
    buff := Malloc(size)
    C.randombytes_buf(buff.ptr, buff.size)
    return buff
  }
  return nil
}
