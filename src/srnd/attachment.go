//
// attachment.go -- nntp attachements
//

package srnd

import (
  "io"
)

type NNTPAttachment interface {
  // the name of the file
  Filename() string
  // path to unpacked file on filesystem
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
}

type nntpAttachment struct {
  ext string
  mime string
  filename string
  filepath string
}


type AttachmentSaver interface {
  // save an attachment given its original filename
  // pass in a reader that reads the content
  Save(filename string, r io.Reader) error
}
