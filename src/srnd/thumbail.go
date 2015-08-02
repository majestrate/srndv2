//
// thumbnail.go -- attachment thumbnails
//

package srnd

// defines an interface for types that can create thumbnails
type ThumbnailGenerator interface {
  // do we accept this mime type?
  AcceptsMime(mime string) bool
  // generate a thumbnail from a reader
  GenerateThumbnail(infname string) error
}
