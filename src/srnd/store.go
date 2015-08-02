//
// store.go
//

package srnd

import (
  "bytes"
  "crypto/sha512"
  "encoding/base32"
  "fmt"
  "io"
  //"io/ioutil"
  "log"
  "mime"
  "mime/multipart"
  "net/mail"
  "net/textproto"
  "os"
  "path/filepath"
  "strings"
)

type ArticleStore interface {
  MessageReader
  MessageWriter
  MessageVerifier
  AttachmentReader
  
  // get the filepath for an attachment
  AttachmentFilepath(fname string) string
  // get the filepath for an attachment's thumbnail
  ThumbnailFilepath(fname string) string
  // do we have this article?
  HasArticle(msgid string) bool
  // create a file for a message
  CreateFile(msgid string) io.WriteCloser
  // create a file for a temp message
  CreateTempFile(msgid string) io.WriteCloser
  // get the filename of a message
  GetFilename(msgid string) string
  // get the filename of a temp message
  GetTempFilename(msgid string) string
  // Get a message given its messageid
  GetMessage(msgid string) NNTPMessage
  // get a temp message given its messageid
  // temp message is deleted once read
  ReadTempMessage(msgid string) NNTPMessage
  // store a post
  StorePost(nntp NNTPMessage) error
  // get article headers only
  GetHeaders(msgid string) ArticleHeaders
}
type articleStore struct {
  directory string
  temp string
  attachments string
  thumbs string
  database Database
}


func createArticleStore(config map[string]string, database Database) ArticleStore {
  store := articleStore{
    directory: config["store_dir"],
    temp: config["incoming_dir"],
    attachments: config["attachments_dir"],
    thumbs: config["thumbs_dir"],
    database: database,
  }
  store.Init()
  return store
}

// initialize article store
func (self articleStore) Init() {
  EnsureDir(self.directory)
  EnsureDir(self.temp)
  EnsureDir(self.attachments)
  EnsureDir(self.thumbs)
}

func (self articleStore) StorePost(nntp NNTPMessage) (err error) {
  f := self.CreateFile(nntp.MessageID())
  if f != nil {
    err = self.WriteMessage(nntp, f)
    f.Close()
  }
  for _, att := range nntp.Attachments() {
    log.Println("save attachment", att.Filename(), "to", att.Filepath())
    fpath := att.Filepath()
    f, err = os.Create(fpath)
    if err == nil {
      err = att.WriteTo(f)
      f.Close()
    }
    if err != nil {
      return
    }
  }
  return 
}

func (self articleStore) ReadMessage(r io.Reader) (NNTPMessage, error) {

  msg, err := mail.ReadMessage(r)
  nntp := nntpArticle{}

  if err == nil {
    nntp.headers = ArticleHeaders(msg.Header)
    log.Println("reading message")
    content_type := msg.Header.Get("Content-Type")
    _, params, err := mime.ParseMediaType(content_type)
    if err != nil {
      log.Println("failed to parse media type", err)
      return nil, err
    }
    boundary, ok := params["boundary"]
    if ok {
      partReader := multipart.NewReader(msg.Body, boundary)
      for {
        part, err := partReader.NextPart()
        if err == io.EOF {
          return nntp, nil
        } else if err == nil {
          hdr := part.Header
          // get content type of part
          part_type := hdr.Get("Content-Type")
          log.Println("part has content type", part_type)
          // parse content type
          media_type, _, err := mime.ParseMediaType(part_type)
          if err == nil {
            if media_type == "text/plain" {
              // plaintext gets added to message part
              nntp.message.body.ReadFrom(part)
              if nntp.message.header == nil {
                nntp.message.header = make(textproto.MIMEHeader)
                nntp.message.header.Set("Content-Type", part_type)
              }
            } else {
              // non plaintext gets added to attachments
              att := self.ReadAttachmentFromMimePart(part)
              nntp = nntp.Attach(att).(nntpArticle)
            }
          } else {
            log.Println("part has no content type", err)
          }
          part.Close()
        } else {
          log.Println("failed to load part! ", err)
          return nntp, err
        }
      }
    } else {   
      _, err = nntp.message.body.ReadFrom(msg.Body)
    }
  } else {
    log.Println("failed to read message", err)
  }
  return nntp, err
}


func (self articleStore) WriteMessage(nntp NNTPMessage, wr io.Writer) (err error) {
  // write headers
  for hdr, hdr_vals := range(nntp.Headers()) {
    for _ , hdr_val := range hdr_vals {
      _, err = io.WriteString(wr, fmt.Sprintf("%s: %s\n", hdr, hdr_val))
      if err != nil {
        return
      }
    }
  }
  // done headers
  _, err = io.WriteString(wr, "\n")
  if err != nil {
    return
  }
  // write body
  err = nntp.WriteBody(wr)
  return
}



func (self articleStore) VerifyMessage(nntp NNTPMessage) (NNTPMessage, error) {
  // TODO: implement
  return nntp, nil
}

// get the filepath for an attachment
func (self articleStore) AttachmentFilepath(fname string) string {
  return filepath.Join(self.attachments, fname)
}

// get the filepath for a thumbanil
func (self articleStore) ThumbnailFilepath(fname string) string {
  return filepath.Join(self.thumbs, fname)
}

// create a file for this article
func (self articleStore) CreateFile(messageID string) io.WriteCloser {
  fname := self.GetFilename(messageID)
  file, err := os.Create(fname)
  if err != nil {
    log.Println("cannot open file", fname)
    return nil
  }
  return file
}

// create a temp file for inboud articles
func (self articleStore) CreateTempFile(messageID string) io.WriteCloser {
  fname := self.GetTempFilename(messageID)
  file, err := os.Create(fname)
  if err != nil {
    log.Println("cannot open file", fname)
    return nil
  }
  return file
}

// return true if we have an article
func (self articleStore) HasArticle(messageID string) bool {
  return self.database.HasArticle(messageID)
}

// get the filename for this article
func (self articleStore) GetFilename(messageID string) string {
  return filepath.Join(self.directory, messageID)
}

// get the filename for this article
func (self articleStore) GetTempFilename(messageID string) string {
  return filepath.Join(self.temp, messageID)
}

// loads temp message and deletes old article
func (self articleStore) ReadTempMessage(messageID string) NNTPMessage {
  fname := self.GetTempFilename(messageID)
  nntp := self.readfile(fname)
  //DelFile(fname)
  return nntp
}

// read a file give filepath
func (self articleStore) readfile(fname string) NNTPMessage {
  
  file, err := os.Open(fname)
  if err != nil {
    log.Println("store cannot open file",fname)
    return nil
  }
  message, err := self.ReadMessage(file)
  file.Close()
  if err == nil {
    return message
  }
  
  log.Println("failed to load file", fname)
  return nil
}

// get the replies for a thread
func (self articleStore) GetThreadReplies(messageID string, last int) []NNTPMessage {
  var repls []NNTPMessage
  if self.database.ThreadHasReplies(messageID) {
    rpls := self.database.GetThreadReplies(messageID, last)
    if rpls == nil {
      return repls
    }
    for _, rpl := range rpls {
      msg := self.GetMessage(rpl)
      if msg == nil {
        log.Println("cannot get message", rpl)
      } else { 
        repls = append(repls, msg)
      }
    }
  }
  return repls
}

// load an article
// return nil on failure
func (self articleStore) GetMessage(messageID string) NNTPMessage {
  return self.readfile(self.GetFilename(messageID))
}

// get article with headers only
func (self articleStore) GetHeaders(messageID string) ArticleHeaders {
  // TODO: don't load the entire body
  nntp := self.readfile(self.GetFilename(messageID))
  if nntp == nil {
    return nil
  }
  return nntp.Headers()
}

func (self articleStore) ReadAttachmentFromMimePart(part *multipart.Part) NNTPAttachment {
  var buff bytes.Buffer
  content_type := part.Header.Get("Content-Type")
  if content_type == "" {
    content_type = "unknown"
  }
  fname := part.FileName()
  idx := strings.LastIndex(fname, ".")
  ext := ".txt"
  if idx > 0 {
    ext = fname[idx:]
  }
  
  n, err := io.Copy(&buff, part)
  log.Printf("read %d bytes from mime part", n)
  if err != nil {
    log.Println("failed to read attachment from mimepart", err)
    return nil
  }
  sha := sha512.Sum512(buff.Bytes())
  hashstr := base32.StdEncoding.EncodeToString(sha[:])
  fpath_fname := hashstr+ext
  fpath := filepath.Join(self.attachments, fpath_fname)

  hdr := part.Header
  return nntpAttachment{
    header: hdr,
    body: buff,
    mime: content_type,
    filename: fname,
    filepath: fpath,
    ext: ext,
    hash: sha[:],
  }
}
