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
  Pubkey() string
  Reference() string
  
  RenderBody() string
  RenderPost() string

  // truncate body to a certain size
  // return copy
  Truncate(amount int) PostModel
  
}

// interface for models that have a navbar
type NavbarModel interface {

  Navbar() string

}

// for threads
type ThreadModel interface {

  BaseModel
  NavbarModel
  
  OP() PostModel
  Replies() []PostModel
  Board() string
  BoardURL() string
}

// board interface
type BoardModel interface {

  BaseModel
  NavbarModel
  
  Frontend() string
  Name() string
  Threads() []ThreadModel
}

type LinkModel interface {

  Text() string
  LinkURL() string
}
