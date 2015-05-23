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

// board interface
type BoardModel interface {

  BaseModel
  
  RenderNavbar() string
  Frontend() string
  Name() string
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
  
  MessageID() string
  PostHash() string
  PostURL() string
  Frontend() string
  Subject() string
  Name() string
  OP() bool
  Attachment() AttachmentModel
  
}

// for threads
type ThreadModel interface {

  BaseModel
  
  OP() PostModel
  Replies() []PostModel
  AddPost(post PostModel)
}
