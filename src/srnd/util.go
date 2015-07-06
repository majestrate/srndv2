//
// util.go
//

package srnd

import (
  "github.com/majestrate/srndv2/src/nacl"
  "crypto/sha1"
  "encoding/base64"
  "encoding/hex"
  "fmt"
  "io"
  "log"
  "net"
  "os"
  "strings"
  "time"
)

func B64Decode(data string) []byte {
  ba, err := base64.URLEncoding.DecodeString(data)
  if err != nil {
    log.Fatal(err)
  }
  return ba
}

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
    }
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
    log.Printf("bad message ID: %s , invalid char at %d: %c", id, idx, c)
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
  return hex.EncodeToString(nacl.RandBytes(length))[:length]
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
  host, _, err := net.SplitHostPort(name)
  if err == nil {
    ip , err := net.ResolveIPAddr("ip", host)
    if err == nil {
      if ip.IP.IsGlobalUnicast() {
        return ip.IP.String()
      }
    }
  }
  return ""
}
