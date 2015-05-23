//
// templates_impl.go
// template model implementation
//
package srnd

import (
  // "github.com/hoisie/mustache"
  "io"
)

type post struct {
  PostModel

  name string
  subject string
  message string
  message_id string
  path string
}

func PostModelFromMessage(nntp *NNTPMessage) PostModel {
  p :=  post{}
  p.name = nntp.Name
  p.subject = nntp.Subject
  p.message = nntp.Message
  p.path = nntp.Path
  p.message_id = nntp.MessageID
  return p
}

type thread struct {
  ThreadModel
  posts []PostModel
}

func (self thread) RenderTo(wr io.Writer) error {
  return nil
}

func (self thread) AddPost(post PostModel) {
  self.posts = append(self.posts, post)
}

func (self thread) OP() PostModel {
  return self.posts[0]
}

func (self thread) Replies() []PostModel {
  if len(self.posts) > 1 {
    return self.posts[1:]
  }
  return []PostModel{}
}

func NewThreadModel(op PostModel) ThreadModel {
  posts := make([]PostModel, 1)
  posts[0] = op
  th := thread{}
  th.posts = posts
  return th
}
