//
// attachment.go -- nntp attachements
//

package srnd

import (
  "bytes"
  "crypto/sha512"
  "encoding/base32"
  "encoding/base64"
  "io"
  "log"
  "mime"
  "mime/multipart"
  "net/textproto"
  "strings"
)

type NNTPAttachment interface {

  io.Reader
  io.WriterTo
  
  // the name of the file
  Filename() string
  // the filepath to the saved file
  Filepath() string
  // the mime type of the attachment
  Mime() string
  // the file extension of the attachment
  Extension() string
  // get the sha512 hash of the attachment
  Hash() []byte
  // do we need to generate a thumbnail?
  NeedsThumbnail() bool
  // mime header
  Header() textproto.MIMEHeader
  // make into a model
  ToModel(prefix string) AttachmentModel
}

type nntpAttachment struct {
  ext string
  mime string
  filename string
  filepath string
  hash []byte
  header textproto.MIMEHeader
  body bytes.Buffer
}

func (self nntpAttachment) ToModel(prefix string) AttachmentModel {
  return attachment{
    prefix: prefix,
    filepath: self.Filepath(),
    filename: self.Filename(),
  }
}

func (self nntpAttachment) Filename() string {
  return self.filename
}

func (self nntpAttachment) Filepath() string {
  return self.filepath
}

func (self nntpAttachment) Mime() string {
  return self.mime
}

func (self nntpAttachment) Extension() string {
  return self.ext
}

func (self nntpAttachment) WriteTo(wr io.Writer) (int64, error) {  
  return self.body.WriteTo(wr)
}


func (self nntpAttachment) Hash() []byte {
  // hash it if we haven't already
  if self.hash == nil || len(self.hash) == 0 {
    h := sha512.Sum512(self.body.Bytes())
    self.hash = h[:]
  }
  return self.hash
}

// TODO: detect
func (self nntpAttachment) NeedsThumbnail() bool {
  for _, ext := range []string{".png", ".jpeg", ".jpg", ".gif", ".bmp", ".webm", ".mp4", ".avi", ".mpeg", ".mpg", ".ogg", ".mp3", ".oga", ".opus", ".flac", ".ico"} {
    if ext == strings.ToLower(self.ext) {
      return true
    }
  }
  return false
}

func (self nntpAttachment) Header() textproto.MIMEHeader {
  return self.header
}

func (self nntpAttachment) Read(d []byte) (n int, err error) {
  return self.body.Read(d)
}


type AttachmentSaver interface {
  // save an attachment given its original filename
  // pass in a reader that reads the content of the attachment
  Save(filename string, r io.Reader) error
}


// create a plaintext attachment
func createPlaintextAttachment(msg string) nntpAttachment {
  buff := new(bytes.Buffer)
  _, _ = io.WriteString(buff, msg)
  header := make(textproto.MIMEHeader)
  mime := "text/plain; charset=UTF-8"
  header.Set("Content-Type", mime)
  return nntpAttachment{
    mime: mime,
    ext: ".txt",
    body: *buff,
    header: header,
  }
}


func createAttachment(content_type, fname string, body io.Reader) NNTPAttachment {
  
  media_type, _, err := mime.ParseMediaType(content_type)
  if err == nil {
    a := nntpAttachment{}
    _, err = io.Copy(&a.body, body)
    if err == nil {
      a.header = make(textproto.MIMEHeader)
      a.mime = media_type + "; charset=UTF-8"
      idx := strings.LastIndex(fname, ".")
      a.ext = ".txt"
      if idx > 0 {
        a.ext = fname[idx:]
      }
      a.header.Set("Filename", fname)
      a.header.Set("Content-Type", a.mime)
      a.header.Set("Content-Transfer-Encoding", "base64")
      h := sha512.Sum512(a.body.Bytes())
      hashstr := base32.StdEncoding.EncodeToString(h[:])
      a.hash = h[:]
      a.filepath = hashstr+a.ext
      a.filename = fname
      return a
    }
  }
  return nil  
}


func readAttachmentFromMimePart(part *multipart.Part) NNTPAttachment {
  hdr := part.Header

  content_type := hdr.Get("Content-Type")
  media_type, _ , err := mime.ParseMediaType(content_type)
  buff := new(bytes.Buffer)
  fname := part.FileName()
  idx := strings.LastIndex(fname, ".")
  ext := ".txt"
  if idx > 0 {
    ext = fname[idx:]
  }

  transfer_encoding := hdr.Get("Content-Transfer-Encoding")
  
  if transfer_encoding == "base64" {
    // read the attachment entirely
    io.Copy(buff, part)
    // clear reference
    part = nil
    // allocate a buffer for the decoded part
    att_bytes := make([]byte, base64.StdEncoding.DecodedLen(buff.Len()))
    decoded_bytes := make([]byte, len(att_bytes))
    // decode
    _, err = base64.StdEncoding.Decode(decoded_bytes, buff.Bytes())
    // reset original attachment buffer
    buff.Reset()
    // copy and wrap
    copy(att_bytes, decoded_bytes)
    buff = bytes.NewBuffer(att_bytes)
    att_bytes = nil
    // clear reference
    decoded_bytes = nil
  } else {
    _, err = io.Copy(buff, part)
    // clear reference
    part = nil
  }
  if err != nil {
    log.Println("failed to read attachment from mimepart", err)
    return nil
  }
  sha := sha512.Sum512(buff.Bytes())
  hashstr := base32.StdEncoding.EncodeToString(sha[:])
  fpath := hashstr+ext
  return nntpAttachment{
    body: *buff,
    header: hdr,
    mime: media_type,
    filename: fname,
    filepath: fpath,
    ext: ext,
    hash: sha[:],
  }
}
