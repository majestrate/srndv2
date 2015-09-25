//
// util.go -- various utilities
//

package srnd

import (
  "github.com/majestrate/srndv2/src/nacl"
  "crypto/sha1"
  "crypto/sha512"
  "encoding/base64"
  "encoding/hex"
  "fmt"
  "io"
  "log"
  "net"
  "os"
  "path/filepath"
  "strconv"
  "strings"
  "time"
)

func DelFile(fname string) {
  if CheckFile(fname) {
    os.Remove(fname)
  }
}

func CheckFile(fname string) bool {
  if _, err := os.Stat(fname) ; os.IsNotExist(err) {
    return false
  }
  return true
}

func IsDir(dirname string) bool {
  stat, err := os.Stat(dirname)
  if err != nil {
    log.Fatal(err)
  }
  return stat.IsDir()
}

// ensure a directory exists
func EnsureDir(dirname string) {
  stat, err := os.Stat(dirname)
  if os.IsNotExist(err) {
    os.Mkdir(dirname, 0755)
  } else if ! stat.IsDir() {
    os.Remove(dirname)
    os.Mkdir(dirname, 0755)
  }
}

// TODO make this work better
func ValidMessageID(id string) bool {
  id_len := len(id)

  if id_len < 5 {
    return false 
  }

  at_idx := strings.Index(id, "@")
  if at_idx < 3 {
    return false
  }
  
  for idx, c := range id {
    if idx == 0 {
      if c == '<' {
        continue
      }
    } else if idx == id_len - 1 {
      if c == '>' {
        continue
      }
    } else {
      if idx == at_idx {
        continue
      }
      if c >= 'a' && c <= 'z' {
        continue
      }
      if c >= 'A' && c <= 'Z' {
        continue
      }
      if c >= '0' && c <= '9' {
        continue
      }
      if c == '.' {
        continue
      }
    }
    log.Printf("bad message ID: len=%d %s , invalid char at %d: %c", id_len, id, idx, c)
    return false
  }
  return true
}

// message id hash
func HashMessageID(msgid string) string {
  return fmt.Sprintf("%x", sha1.Sum([]byte(msgid)))
}
// short message id hash
// >>hash
func ShortHashMessageID(msgid string) string {
  return HashMessageID(msgid)[:10]
}

type lineWriter struct {
  io.Writer
  wr io.Writer
  delim []byte
}

func NewLineWriter(wr io.Writer, delim string) io.Writer {
  return lineWriter{wr, wr, []byte(delim)}
}

func (self lineWriter) Write(data []byte) (n int, err error) {
  n, err = self.wr.Write(data)
  self.wr.Write(self.delim)
  return n, err
}

func OpenFileWriter(fname string) (io.WriteCloser, error) {
  return os.Create(fname)
}

// make a random string
func randStr(length int) string {
  return hex.EncodeToString(nacl.RandBytes(length))[length:]
}


// time for right now as int64
func timeNow() int64 {
  return time.Now().Unix()
}

// sanitize data for nntp
func nntpSanitize(data string) string {
  parts := strings.Split(data, "\n.\n")
  return parts[0]
}


type int64Sorter []int64

func (self int64Sorter) Len() int {
  return len(self)
}

func (self int64Sorter) Less(i, j int) bool {
  return self[i] < self[j]
}


func (self int64Sorter) Swap(i, j int) {
  tmp := self[j]
  self[j] = self[i]
  self[i] = tmp
}


// obtain the "real" ip address
func getRealIP(name string) string {
  if len(name) > 0 {
    ip , err := net.ResolveIPAddr("ip", name)
    if err == nil {
      if ip.IP.IsGlobalUnicast() {
      return ip.IP.String()
      }
    }
  }
  return ""
}

// check that we have permission to access this
// fatal on fail
func checkPerms(fname string) {
  fstat, err := os.Stat(fname)
  if err != nil {
    log.Fatalf("Cannot access %s, %s", fname, err)
  }
  // check if we can access this dir
  if fstat.IsDir() {
    tmpfname := filepath.Join(fname, ".test")
    f, err := os.Create(tmpfname)
    if err != nil {
      log.Fatalf("No Write access in %s, %s", fname, err)
    }
    err = f.Close()
    if err != nil {
      log.Fatalf("failed to close test file %s !? %s", tmpfname, err)
    }
    err = os.Remove(tmpfname)
    if err != nil {
      log.Fatalf("failed to remove test file %s, %s", tmpfname, err)
    }
  } else {
    // this isn't a dir, treat it like a regular file
    f, err := os.Open(fname)
    if err != nil {
      log.Fatalf("cannot read file %s, %s", fname, err)
    }
    f.Close()
  }
}

// given an address
// generate a new encryption key for it
// return the encryption key and the encrypted address
func newAddrEnc(addr string) (string, string) {
  key_bytes := nacl.RandBytes(64)
  key := base64.StdEncoding.EncodeToString(key_bytes)
  return key, encAddr(addr, key)
}

// xor address with a one time pad
// if the address isn't long enough it's padded with spaces
func encAddr(addr, key string) string {
  key_bytes, err := base64.StdEncoding.DecodeString(key)

  if err != nil {
    log.Println("encAddr() key base64 decode", err)
    return ""
  }
  
  if len(addr) > len(key_bytes) {
    log.Println("encAddr() len(addr) > len(key_bytes)")
    return ""
  }
  
  // pad with spaces
  for len(addr) < len(key_bytes) {
    addr += " "
  }

  addr_bytes := []byte(addr)
  res_bytes := make([]byte, len(addr_bytes))
  for idx, b := range key_bytes {
    res_bytes[idx] = addr_bytes[idx] ^ b
  }
  
  return base64.StdEncoding.EncodeToString(res_bytes)
}

// decrypt an address
// strips any whitespaces
func decAddr(encaddr, key string) string {
  encaddr_bytes, err := base64.StdEncoding.DecodeString(encaddr)
  if err != nil {
    log.Println("decAddr() encaddr base64 decode", err)
    return ""
  }
  if len(encaddr_bytes) != len(key) {
    log.Println("decAddr() len(encaddr_bytes) != len(key)")
    return ""
  }
  key_bytes, err := base64.StdEncoding.DecodeString(key)
  if err != nil {
    log.Println("decAddr() key base64 decode", err)
  }
  res_bytes := make([]byte, len(key))
  for idx, b := range key_bytes {
    res_bytes[idx] = encaddr_bytes[idx] ^ b
  }
  res := string(res_bytes)
  return strings.Trim(res, " ")
}


func newsgroupValidFormat(newsgroup string) bool {
  // too long newsgroup
  if len(newsgroup) > 128 {
    return false
  }
  for _, ch := range newsgroup {
    if ch >= 'a' && ch <= 'z' {
      continue
    }
    if ch >= '0' && ch <= '9' {
      continue
    }
    if ch >= 'A' && ch <= 'Z' {
      continue
    }
    if ch == '.' {
      continue
    }
    return false
  }
  return true
}


// generate a new signing keypair
// public, secret
func newSignKeypair() (string, string) {
  kp := nacl.GenSignKeypair()
  defer kp.Free()
  pk := kp.Public()
  sk := kp.Secret()
  return hex.EncodeToString(pk), hex.EncodeToString(sk)
}

// make a utf-8 tripcode
func makeTripcode(pk string) string {
  data, err := hex.DecodeString(pk)
  if err == nil {
    tripcode := ""
    //  here is the python code this is based off of
    //  i do something slightly different but this is the base
    //
    //  for x in range(0, length / 2):
    //    pub_short += '&#%i;' % (9600 + int(full_pubkey_hex[x*2:x*2+2], 16))
    //  length -= length / 2
    //  for x in range(0, length):
    //    pub_short += '&#%i;' % (9600 + int(full_pubkey_hex[-(length*2):][x*2:x*2+2], 16))
    //
    for _, c := range data {
      ch := 9600
      ch += int(c)
      tripcode += fmt.Sprintf("&#%04d;", ch)
    }
    return tripcode
  }
  return "[invalid]"
}

// generate a new message id with base name
func genMessageID(name string) string {
  return fmt.Sprintf("<%s%d@%s>", randStr(5), timeNow(), name)
}

// time now as a string timestamp
func timeNowStr() string {
  return time.Unix(timeNow(), 0).UTC().Format(time.RFC1123Z)
}

// get from a map an int given a key or fall back to a default value
func mapGetInt(m map[string]string, key string, fallback int) int {
  val, ok := m[key]
  if ok {
    i, err := strconv.ParseInt(val, 10, 32)
    if err == nil {
      return int(i)
    }
  } 
  return fallback
}

func isSage(str string) bool {
  str = strings.ToLower(str)
  return str == "sage" || strings.HasPrefix(str, "sage ")
}

func unhex(str string) []byte {
  buff, _ := hex.DecodeString(str)
  return buff
}

func hexify(data []byte) string {
  return hex.EncodeToString(data)
}

// extract pubkey from secret key
// return as base32
func getSignPubkey(sk []byte) string {
  return hexify(nacl.GetSignPubkey(sk))
}

// sign data with secret key the fucky srnd way
// return signature as base32
func cryptoSign(data, sk []byte) string {
  // hash
  hash := sha512.Sum512(data)
  log.Printf("hash=%s len=%s", hexify(hash[:]), len(data))
  // sign
  sig := nacl.CryptoSignFucky(hash[:], sk)
  return hexify(sig)
}

// given a tripcode after the #
// make a seed byteslice
func parseTripcodeSecret(str string) []byte {
  // try decoding hex
  raw := unhex(str)
  keylen := nacl.CryptoSignSeedLen()
  if raw == nil || len(raw) != keylen {
    // treat this as a "regular" chan tripcode
    // decode as bytes then pad the rest with 0s if it doesn't fit
    raw = make([]byte, keylen)
    str_bytes := []byte(str)
    if len(str_bytes) > keylen {
      copy(raw, str_bytes[:keylen])
    } else {
      copy(raw, str_bytes)
    }
  } 
  return raw
}
