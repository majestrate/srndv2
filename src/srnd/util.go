//
// util.go
//

package srnd

import (
  "crypto/sha1"
  "encoding/base64"
  "fmt"
  "io"
  "log"
  "os"
  "strings"
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
  if len(id) <= 4 {
    return false 
  }
  if id[0] != '<' || id[len(id)-1] != '>' {
    log.Println(id[0], id[len(id)-1])
    return false
  }
  if strings.Count(id, "@") != 1 {
    return false
  }
  if strings.Count(id, "/") > 0 {
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
