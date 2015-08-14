//
// attachment.go -- nntp attachements
//

package srnd

import (
  "bytes"
  "crypto/sha512"
  "io"
  "net/textproto"
  "strings"
)

type NNTPAttachment interface {
  // the name of the file
  Filename() string
  // the filepath to the saved file
  Filepath() string
  // the mime type of the attachment
  Mime() string
  // the file extension of the attachment
  Extension() string
  // write this attachment out to a writer
  WriteTo(wr io.Writer) error
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
    source: prefix+"img/"+self.Filepath(),
    thumbnail: prefix+"thm/"+self.Filepath(),
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

func (self nntpAttachment) WriteTo(wr io.Writer) error {  
  _, err := self.body.WriteTo(wr)
  return err
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
  return strings.HasPrefix(self.mime, "image/")
}

func (self nntpAttachment) Header() textproto.MIMEHeader {
  return self.header
}


type AttachmentSaver interface {
  // save an attachment given its original filename
  // pass in a reader that reads the content of the attachment
  Save(filename string, r io.Reader) error
}


// create a plaintext attachment
func createPlaintextAttachment(msg string) nntpAttachment {
  var buff bytes.Buffer
  _, _ = io.WriteString(&buff, msg)
  header := make(textproto.MIMEHeader)
  mime := "text/plain; charset=UTF-8"
  header.Set("Content-Type", mime)
  return nntpAttachment{
    mime: mime,
    ext: ".txt",
    body: buff,
    header: header,
  }
}
