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


// take in a json object from the liveui and make it into an nntp attachment
func createAttachmentFromJSON(j interface{}) NNTPAttachment {

  var att map[string]interface{}
  switch j.(type) {
  default:
    log.Println("invalid json for attachment")
    return nil
  case map[string]interface{}:
    att = j.(map[string]interface{})
  }

  var fname_str, content_type_str string

  fname, _ := att["filename"]
  switch fname.(type) {
  default:
    log.Println("invalid filename type for attachment")
    return nil
  case string:
    fname_str = fname.(string)
    break
  }
  
  idx := strings.LastIndex(fname_str, ".")
  ext := ".txt"
  if idx > 0 {
    ext = fname_str[idx:]
  }

  content_type, _ := att["content_type"]
  switch content_type.(type) {
  default:
    log.Println("invalid content-type for attachment")
    return nil
  case string:
    content_type_str = content_type.(string)
    break
  }
  
 
  media_type, _ , err := mime.ParseMediaType(content_type_str)
  var data []byte
  if err == nil {
    buff := new(bytes.Buffer)
    body, _ := att["data"]
    switch body.(type) {
    default:
      log.Println("invalid attachment type")
      return nil
    case string:
      data, err = base64.StdEncoding.DecodeString(body.(string))
    }
    if err == nil {
      buff.Write(data)
      sha := sha512.Sum512(data)
      hashstr := base32.StdEncoding.EncodeToString(sha[:])
      fpath_str := hashstr+ext
      return nntpAttachment{
        body: *buff,
        mime: media_type,
        filename: fname_str,
        filepath: fpath_str,
        ext: ext,
        hash: sha[:],
      }
    } else {
      log.Println("failed to decode attachment data", err)
    }
  } else {
    log.Println("invalid content type", content_type_str)
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
