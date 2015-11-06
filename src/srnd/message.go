//
// message.go
//
package srnd

import (
  "github.com/majestrate/srndv2/src/nacl"
  "bufio"
  "bytes"
  "encoding/base64"
  "errors"
  "fmt"
  "io"
  "log"
  "mime"
  "mime/multipart"
  "net/textproto"
  "strings"
  "time"
)


type ArticleHeaders map[string][]string

func (self ArticleHeaders) Has(key string) bool {
  _, ok := self[key]
  return ok
}

func (self ArticleHeaders) Set(key, val string) {
  self[key] = []string{val}
}

func (self ArticleHeaders) Get(key, fallback string) string {
  val , ok := self[key]
  if ok {
    return val[0]
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
  // write out everything
  WriteTo(wr io.Writer, delim string) error
  // write out body
  WriteBody(wr io.Writer, delim string) error
  // attach a file
  Attach(att NNTPAttachment) NNTPMessage
  // get the plaintext message if it exists
  Message() string
  // pack the whole message and prepare for write
  Pack()
  // get the inner nntp article that is signed and valid, returns nil if not present or invalid
  Signed() NNTPMessage
  // get the pubkey for this message if it was signed, otherwise empty string
  Pubkey() string
  // get the origin encrypted address, i2p destination or empty string for onion posters
  Addr() string
}

type MessageReader interface {
  // read a message from a reader
  ReadMessage(r io.Reader) (NNTPMessage, error)
}

type MessageWriter interface {
  // write a message to a writer
  WriteMessage(nntp NNTPMessage, wr io.Writer) error
}

type nntpArticle struct {
  // mime header
  headers ArticleHeaders
  // multipart boundary
  boundary string
  // the text part of the message
  message nntpAttachment
  // any attachments
  attachments []NNTPAttachment
  // the inner nntp message to be verified
  signedPart nntpAttachment
}

// create a simple plaintext nntp message
func newPlaintextArticle(message, email, subject, name, instance, message_id,  newsgroup string) NNTPMessage {
  nntp := nntpArticle{
    headers: make(ArticleHeaders),
  }
  nntp.headers.Set("From", fmt.Sprintf("%s <%s>", name, email))
  nntp.headers.Set("Subject", subject)
  if isSage(subject) {
    nntp.headers.Set("X-Sage", "1")
  }
  nntp.headers.Set("Path", instance)
  nntp.headers.Set("Message-ID", message_id)
  // posted now
  nntp.headers.Set("Date", timeNowStr())
  nntp.headers.Set("Newsgroups", newsgroup)
  nntp.message = createPlaintextAttachment(message)
  nntp.Pack()
  return nntp
}

// sign an article with a seed
func signArticle(nntp NNTPMessage, seed []byte) (signed nntpArticle, err error) {
  signed.headers = make(ArticleHeaders)
  h := nntp.Headers()
  // copy headers
  // copy into signed part
  for k := range(h) {
    if k == "Content-Type" {
      signed.headers.Set(k, "message/rfc822; charset=UTF-8")
    } else {
      v := h[k][0]
      signed.headers.Set(k, v)
    }
  }
  signbuff := new(bytes.Buffer)
  // write body to sign buffer
  err = nntp.WriteTo(signbuff, "\r\n")
  if err == nil {
    // build keypair
    kp := nacl.LoadSignKey(seed)
    if kp == nil {
      log.Println("failed to load seed for signing article")
      return
    }
    defer kp.Free()
    sk := kp.Secret()
    pk := getSignPubkey(sk)
    // sign it nigguh
    data := signbuff.Bytes()
    sig := cryptoSign(data, sk)
    // log that we signed it
    log.Printf("signed %s pubkey=%s sig=%s", nntp.MessageID(), pk, sig)
    signed.headers.Set("X-Signature-Ed25519-SHA512", sig)
    signed.headers.Set("X-PubKey-Ed25519", pk)
  }
  // copy sign buffer into signed part
  _, err = io.Copy(&signed.signedPart.body, signbuff)
  // add this so the writer writes the entire post
  signed.signedPart.body.Write([]byte{13,10})
  return 
}

func (self nntpArticle) WriteTo(wr io.Writer, delim string) (err error) {
  // write headers
  for hdr, hdr_vals := range(self.Headers()) {
    for _ , hdr_val := range hdr_vals {
      _, err = io.WriteString(wr, fmt.Sprintf("%s: %s%s", hdr, hdr_val, delim))
      if err != nil {
        log.Println("error while writing headers", err)
        return
      }
    }
  }
  // done headers
  _, err = io.WriteString(wr, delim)
  if err != nil {
    log.Println("error while writing body", err)
    return
  }

  // write body
  err = self.WriteBody(wr, delim)
  return
}
  
func (self nntpArticle) Pubkey() string {
  return self.headers.Get("X-PubKey-Ed25519", self.headers.Get("X-Pubkey-Ed25519" , ""))
}

func (self nntpArticle) Signed() NNTPMessage {
  if self.signedPart.body.Len() > 0 {
    log.Println("loading signed message")
    msg, err := read_message(&self.signedPart.body)
    if err == nil {
      return msg
    }
    log.Println("failed to load signed message", err)
  }
  return nil
}

func (self nntpArticle) MessageID() (msgid string) {
  for _, h := range []string{"Message-ID", "Messageid", "MessageID","Message-Id"} {
    msgid = self.headers.Get(h, "")
    if msgid != "" {
      return
    }
  }
  return
}

func (self nntpArticle) Pack() {
  if len(self.attachments) > 0 {
    if len(self.boundary) == 0 {
      // we have no boundry, set it
      self.boundary = randStr(24)
      // set headers
      self.headers.Set("Mime-Version", "1.0")
      self.headers.Set("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%s", self.boundary))
    }
  } else if self.signedPart.body.Len() == 0 {
    self.headers.Set("Content-Type", "text/plain; charset=utf-8")
  }
    
}

func (self nntpArticle) Reference() string {
  return self.headers.Get("Reference", self.headers.Get("References",""))
}

func (self nntpArticle) Newsgroup() string {
  return self.headers.Get("Newsgroups", "")
}

func (self nntpArticle) Name() string {
  from := self.headers.Get("From", "anonymous <a@no.n>")
  idx := strings.Index(from, "<")
  if idx > 1 {
    return from[:idx]
  }
  return "[Invalid From header]"
}

func (self nntpArticle) Addr() (addr string) {
  addr = self.headers.Get("X-Encrypted-Ip", "")
  if addr != "" {
    return
  }

  addr = self.headers.Get("X-Encrypted-IP", "")
  if addr != "" {
    return
  }

  addr = self.headers.Get("X-I2P-DestHash", "")
  if addr != "" {
    if addr == "None" {
      return ""
    }
    return
  }
  
  addr = self.headers.Get("X-I2p-Desthash", "")
  return
}

func (self nntpArticle) Email() string {
  from := self.headers.Get("From", "anonymous <a@no.n>")
  idx := strings.Index(from, "<")
  if idx > 1 {
    return from[:idx-1]
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
  return strings.Trim(self.message.body.String(), "\x00")
}

func (self nntpArticle) Path() string {
  return self.headers.Get("Path", "unspecified")
}

func (self nntpArticle) Headers() ArticleHeaders {
  return self.headers
}

func (self nntpArticle) AppendPath(part string) NNTPMessage {
  if self.headers.Has("Path") {
    self.headers.Set("Path", part + "!" + self.Path())
  } else {
    self.headers.Set("Path", part)
  }
  return self
}
func (self nntpArticle) ContentType() string {
  // assumes text/plain if unspecified
  return self.headers.Get("Content-Type", "text/plain; charset=UTF-8")
}

func (self nntpArticle) Sage() bool {
  return self.headers.Get("X-Sage", "") == "1"
}

func (self nntpArticle) OP() bool {
  return self.headers.Get("Reference", self.headers.Get("References", "")) == ""
}

func (self nntpArticle) Attachments() []NNTPAttachment {
  return self.attachments
}


func (self nntpArticle) Attach(att NNTPAttachment) NNTPMessage {
  self.attachments = append(self.attachments, att)
  return self
}

func (self nntpArticle) WriteBody(wr io.Writer, delim string) (err error) {
  // this is a signed message, don't treat it special
  if self.signedPart.body.Len() > 0 {    
    if delim ==  "\r\n" {
      // delimiter is \r\n
      // for signing we copy verbatum
      // TODO: do we cut off the last line ending?
      _, err = io.Copy(wr, &self.signedPart.body)
    } else if delim == "\n" {
      // assumes signedpart is in \r\n
      r := bufio.NewReader(&self.signedPart.body)
      w := bufio.NewWriter(wr)
      var line []byte
      for {
        // convert line endings :\
        line, err = r.ReadBytes(10)
        if err == nil {
          if len(line) > 2 {
            _, err = w.Write(line[:len(line)-2])
          }
          _, err = w.Write([]byte{10})
          err = w.Flush() // flush it
        } else if err == io.EOF {
          return
        } else {
          log.Println("error while writing", err)
          return
        }
      }
    } else {
      err = errors.New("unsupported delimiter")
      return
    }
  }
  if len(self.attachments) == 0 {
    // write plaintext and be done
    _, err = io.Copy(wr, &self.message.body)
    return
  }
  content_type := self.ContentType()
  _, params, err := mime.ParseMediaType(content_type)
  if err != nil {
    log.Println("failed to parse media type", err)
    return err
  }

  boundary, ok := params["boundary"]
  if ok {
    w := multipart.NewWriter(wr)
    
    err = w.SetBoundary(boundary)
    if err == nil {
      attachments := []NNTPAttachment{self.message}
      attachments = append(attachments, self.attachments...)
      for _ , att := range(attachments) {
        hdr := att.Header()
        if hdr == nil {
          hdr = make(textproto.MIMEHeader)
        }
        hdr.Set("Content-Transfer-Encoding", "base64")
        part, err := w.CreatePart(hdr)
        enc := base64.NewEncoder(base64.StdEncoding, part)
        _, err = io.Copy(enc, att)
        enc.Close()  

        if err != nil {
          break
        }
      }
    }
    if err != nil {
      log.Println("error writing part", err)
    }
    err = w.Close()
  } else {
    _, err = self.message.body.WriteTo(wr)
  }
  return err
}
