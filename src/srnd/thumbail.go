//
// thumbnail.go -- attachment thumbnails
//

package srnd

import (
  "io"
)

// a generated thumbnail
type Thumbnail struct {
  Filepath string
  Filename string
}

// defines an interface for types that can create thumbnails
type ThumbnailGenerator interface {
  // do we accept this mime type?
  AcceptsMime(mime string) bool
  // generate a thumbnail from a reader
  GenerateFrom(r io.Reader) ( Thumbnail, error )
}
