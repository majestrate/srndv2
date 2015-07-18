//
// message.go
//
package srnd

import (
/*
  "bufio"
  "bytes"
  "crypto/rand"
  "crypto/sha512"
  "encoding/hex"
  "fmt"
  "github.com/majestrate/srndv2/src/nacl"
  "io"
  "log"
  "mime"
  "mime/multipart"
  "net/textproto"
  "path/filepath"
  "time"
*/
  "fmt"
  "io"
  "strings"
  "time"

)


type ArticleHeaders map[string]string

func (self ArticleHeaders) Has(key string) bool {
  _, ok := self[key]
  return ok
}

func (self ArticleHeaders) Get(key, fallback string) string {
  val , ok := self[key]
  if ok {
    return val
  } else {
    return fallback
  }
}


type NNTPMessage interface {
  // this message's messsge id
  MessageID() string
  // the parent message's messageid if it's specified
  Reference() string
  // the newsgroup this post is in
  Newsgroup() string
  // the name of the poster
  Name() string
  // any email address associated with the post
  Email() string
  // the subject of the post
  Subject() string
  // when this was posted
  Posted() int64
  // the message from the poster
  Message() string
  // the path header
  Path() string
  // append something to path
  // return message with new path
  AppendPath(part string) NNTPMessage
  // the type of this message usually a mimetype
  ContentType() string
  // was this post a sage?
  Sage() bool
  // was this post a root post?
  OP() bool
  // all attachments
  Attachments() []NNTPAttachment
  // all headers
  Headers() ArticleHeaders
  // write out body
  WriteBody(wr io.Writer) error
}

type MessageReader interface {
  // read a message from a reader
  ReadMessage(r io.Reader) (NNTPMessage, error)
}

type MessageWriter interface {
  // write a message to a writer
  WriteMessage(nntp NNTPMessage, wr io.Writer) error
}

type MessageSigner interface {
  // sign a message, return a new message that is signed
  SignMessage(nntp NNTPMessage) (NNTPMessage, error)
}

type MessageVerifier interface {
  // verify a message, return the message that is verified
  VerifyMessage(nntp NNTPMessage) (NNTPMessage, error)
}

type nntpArticle struct {
  headers ArticleHeaders
  message string
}

// create a simple plaintext nntp message
func newPlaintextArticle(message, email, subject, name, instance, newsgroup string) NNTPMessage {
  nntp := nntpArticle{make(ArticleHeaders), message}
  nntp.headers["From"] = fmt.Sprintf("%s <%s>", name, email)
  nntp.headers["Subject"] = subject
  nntp.headers["Path"] = instance
  nntp.headers["Content-Type"] = "text/plain"
  nntp.headers["MessageID"] = genMessageID(instance)
  // posted now
  nntp.headers["Posted"] = timeNowStr()
  nntp.headers["Newsgroups"] = newsgroup

  return nntp
}

func (self nntpArticle) MessageID() string {
  return self.headers.Get("MessageID", "")
}

func (self nntpArticle) Reference() string {
  return self.headers.Get("Reference", "")
}

func (self nntpArticle) Newsgroup() string {
  return self.headers.Get("Newsgroups", "")
}

func (self nntpArticle) Name() string {
  from := self.headers.Get("From", "anonymous <a@no.n>")
  idx := strings.Index(from, " ")
  if idx > 1 {
    return from[:idx]
  }
  return "[Invalid From header]"
}

func (self nntpArticle) Email() string {
  from := self.headers.Get("From", "anonymous <a@no.n>")
  idx := strings.Index(from, " ")
  if idx > 1 {
    idx_1 := strings.Index(from[:idx], "<")
    idx_2 := strings.Index(from[:idx], ">")
    if idx_2 > 0 && idx_1 > 0 && idx_2 > idx_1 {
      return from[1+idx+idx_1:idx+idx_2]
    }
  }
  return "[Invalid From header]"
  
}

func (self nntpArticle) Subject() string {
  return self.headers.Get("Subject", "")
}

func (self nntpArticle) Posted() int64 {
  posted := self.headers.Get("Date", "")
  t, err := time.Parse(time.RFC1123Z, posted)
  if err == nil {
    return t.Unix()
  }
  return 0
}

func (self nntpArticle) Message() string {
  return self.message
}

func (self nntpArticle) Path() string {
  return self.headers.Get("Path", "unspecified")
}

func (self nntpArticle) Headers() ArticleHeaders {
  return self.headers
}

func (self nntpArticle) AppendPath(part string) NNTPMessage {
  if self.headers.Has("Path") {
    self.headers["Path"] = part + "!" + self.Path()
  } else {
    self.headers["Path"] = part
  }
  return self
}
func (self nntpArticle) ContentType() string {
  // assumes text/plain if unspecified
  return self.headers.Get("Content-Type", "text/plain")
}

func (self nntpArticle) Sage() bool {
  return self.headers.Has("X-Sage")
}

func (self nntpArticle) OP() bool {
  return ! self.headers.Has("Reference")
}

func (self nntpArticle) Attachments() []NNTPAttachment {
  // TODO: implement?
  return nil
}


func (self nntpArticle) WriteBody(wr io.Writer) (err error) {
  // TODO: attachments
  _, err = io.WriteString(wr, self.message)
  return
}
