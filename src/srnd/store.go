//
// store.go
//

package srnd

import (
  "github.com/streadway/amqp"
  "github.com/majestrate/srndv2/src/nacl"
  "github.com/gographics/imagick/imagick"
  "bufio"
  "bytes"
  "crypto/sha512"
  "errors"
  "fmt"
  "io"
  "log"
  "mime"
  "mime/multipart"
  "net/mail"
  "os"
  "path/filepath"
)


type ArticleStore interface {
  MessageReader
  MessageWriter
  
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

  // get our temp directory for articles
  TempDir() string
}
type articleStore struct {
  directory string
  temp string
  attachments string
  thumbs string
  database Database
  r_conn *amqp.Connection
  r_chnl *amqp.Channel
  r_q amqp.Queue
}

func createArticleStore(config map[string]string, rmq_url string, database Database) ArticleStore {
  store := articleStore{
    directory: config["store_dir"],
    temp: config["incoming_dir"],
    attachments: config["attachments_dir"],
    thumbs: config["thumbs_dir"],
    database: database,
  }
  store.Init()
  if rmq_url != "" {
    var err error
    store.r_conn, store.r_chnl, err = rabbitConnect(rmq_url)
    if err != nil {
      log.Fatal("failed to connect to rabbitmq message broker", err)
    }
    store.r_q, err = rabbitQueue("srndv2", store.r_chnl)
  }
  return store
}

func (self articleStore) TempDir() string {
  return self.temp
}

// initialize article store
func (self articleStore) Init() {
  EnsureDir(self.directory)
  EnsureDir(self.temp)
  EnsureDir(self.attachments)
  EnsureDir(self.thumbs)
}

// generate thumbnail in same process
func (self articleStore) generateThumbnail(infname string) (err error) {
  wand := imagick.NewMagickWand()
  // read image source
  err = wand.ReadImage(self.AttachmentFilepath(infname))
  if err == nil {
    // get size
    h := wand.GetImageHeight()
    w := wand.GetImageWidth()

    // calculate scale parameters
    var scale, th, tw float64 
    scale = 200
    modifier := scale / float64(w)
    th = modifier * float64(h)
    tw = modifier * float64(w)
    // scale it
    err = wand.ScaleImage(uint(tw), uint(th))
    if err == nil {
      err = wand.WriteImage(self.ThumbnailFilepath(infname))
    }
  }
  // explicitly destroy
  wand.Destroy()
  return
}

// queue thumbnail generation to rabbitmq
func (self articleStore) queueGenerateThumbnail(infname string) (err error) {
  log.Println("queue file for thumbnailing", infname)
  err = self.r_chnl.Publish(
    "",
    "",
    false,
    false,
    amqp.Publishing{
      ContentType: "text/plain",
      Body: []byte(fmt.Sprintf("thumbnail %s %s",self.AttachmentFilepath(infname), self.ThumbnailFilepath(infname))),
      
    })
  return err
}

func (self articleStore) GenerateThumbnail(infname string) (err error) {
  if self.r_conn == nil {
    return self.generateThumbnail(infname)
  } else {
    return self.queueGenerateThumbnail(infname)
  }
}

func (self articleStore) ReadMessage(r io.Reader) (NNTPMessage, error) {
  return read_message(r)
}

func (self articleStore) StorePost(nntp NNTPMessage) (err error) {
  f := self.CreateFile(nntp.MessageID())
  if f != nil {
    err = self.WriteMessage(nntp, f)
    f.Close()
  }

  nntp_inner := nntp.Signed()
  if nntp_inner == nil {
    // no inner article
    // store the data in the article
    self.database.RegisterArticle(nntp)
    for _, att := range nntp.Attachments() {
      // save attachments in parallel
      go self.saveAttachment(att)
    }
  } else {
    // we have inner data
    // store the signed data
    self.database.RegisterArticle(nntp_inner)
    for _, att := range nntp_inner.Attachments() {
      go self.saveAttachment(att)
    }
    // record a tripcode
    self.database.RegisterSigned(nntp.MessageID(), nntp.Pubkey())
  }
  return
}

// save an attachment
func (self articleStore) saveAttachment(att NNTPAttachment) {
  fpath := att.Filepath()
  upload := self.AttachmentFilepath(fpath)
  if CheckFile(upload) {
    log.Println("already have file")
    return
  }
  // save attachment
  log.Println("save attachment", att.Filename(), "to", upload)
  f, err := os.Create(upload)
  if err == nil {
    err = att.WriteTo(f)
    f.Close()
  }
  if err != nil {
    return
  }
  
  // generate thumbanils
  if att.NeedsThumbnail() {
    err = self.GenerateThumbnail(fpath)
    if err != nil {
      log.Println("failed to generate thumbnail", err) 
    }
  }
}

// eh this isn't really needed is it?
func (self articleStore) WriteMessage(nntp NNTPMessage, wr io.Writer) (err error) {
  return nntp.WriteTo(wr, "\n")
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
  DelFile(fname)
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


func read_message(r io.Reader) (NNTPMessage, error) {

  msg, err := mail.ReadMessage(r)
  var nntp nntpArticle

  if err == nil {
    nntp.headers = ArticleHeaders(msg.Header)
    content_type := nntp.ContentType()
    media_type, params, err := mime.ParseMediaType(content_type)
    if err != nil {
      log.Println("failed to parse media type", err, "for mime", content_type)
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
          // parse content type
          media_type, _, err = mime.ParseMediaType(part_type)
          if err == nil {
            if media_type == "text/plain" {
              att := readAttachmentFromMimePart(part)
              nntp.message = att.(nntpAttachment)
              nntp.message.header.Set("Content-Type", part_type)
            } else {
              // non plaintext gets added to attachments
              att := readAttachmentFromMimePart(part)
              if att != nil {
                nntp = nntp.Attach(att).(nntpArticle)
              }
            }
          } else {
            log.Println("part has no content type", err)
          }
          part.Close()
        } else {
          log.Println("failed to load part! ", err)
          return nil, err
        }
      }

    } else if media_type == "message/rfc822" {
      // tripcoded message
      sig := nntp.headers.Get("X-Signature-Ed25519-Sha512", "")
      pk := nntp.Pubkey()
      if pk == "" || sig == "" {
        log.Println("invalid sig or pubkey", sig, pk)
        return nil, errors.New("invalid headers")
      }
      log.Printf("got signed message from %s", pk)
      pk_bytes := unhex(pk)
      sig_bytes := unhex(sig)
      r := bufio.NewReader(msg.Body)
      crlf := []byte{13,10}
      for {
        line, err := r.ReadBytes('\n')
        if err == io.EOF {
          break
        }
        nntp.signedPart.body.Write(line[:len(line)-1])
        nntp.signedPart.body.Write(crlf)
      }
      if nntp.signedPart.body.Len() < 2 {
        log.Println("signed body is too small")
      } else {
        body := nntp.signedPart.body.Bytes()[:nntp.signedPart.body.Len()-2]
        body_hash := sha512.Sum512(body)
        log.Printf("hash=%s", hexify(body_hash[:]))
        if nacl.CryptoVerifyFucky(body_hash[:], sig_bytes, pk_bytes) {
          log.Println("signature is valid :^)")
          return nntp, nil
        } else {
          log.Println("!!!signature is invalid!!!")
        }
      }
    } else {
      // plaintext attachment
      buff := new(bytes.Buffer)
      _, err = io.Copy(buff, msg.Body)
      nntp.message = createPlaintextAttachment(buff.String())
      return nntp, err
    }
  } else {
    log.Println("failed to read message", err)
    return nil, err
  }
  return nntp, err
}

