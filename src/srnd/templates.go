//
// templates.go
// template model interfaces
//
package srnd

import (
  "io"
)

// base model type
type BaseModel interface {

  // site url prefix
  Prefix() string

  // render to a writer
  RenderTo(wr io.Writer) error

}


// for attachments
type AttachmentModel interface {

  BaseModel
  
  Thumbnail() string
  Source() string
  Filename() string
  
}

// for individual posts
type PostModel interface {

  BaseModel

  CSSClass() string
  
  MessageID() string
  PostHash() string
  ShortHash() string
  PostURL() string
  Frontend() string
  Subject() string
  Name() string
  Date() string
  OP() bool
  Attachments() []AttachmentModel
  Board() string
  Sage() bool
  
  RenderBody() string
  RenderPost() string
  
}

// for threads
type ThreadModel interface {

  BaseModel
  
  OP() PostModel
  Replies() []PostModel
  Board() string
}

// board interface
type BoardModel interface {

  BaseModel
  
  RenderNavbar() string
  Frontend() string
  Name() string
  Threads() []ThreadModel
}
