package nacl


// #include <sodium.h>
// #cgo LDFLAGS: -lsodium
import "C"

func randbytes(size C.size_t) *Buffer {

  buff := malloc(size)
  C.randombytes_buf(buff.ptr, size)
  return buff

}

func RandBytes(size int) []byte {
  if size > 0 {
    buff := randbytes(C.size_t(size))
    defer buff.Free()
    return buff.Bytes()
  }
  return nil
}
