//
// frontend.go
// srnd frontend interfaces
//
//
package srnd

// frontend interface for any type of frontend
type Frontend interface {

  // channel that is for the nntpd to poll for new posts from this frontend
  NewPostsChan() chan NNTPMessage

  // channel that is for the frontend to pool for new posts from the nntpd
  PostsChan() chan NNTPMessage
  
  // run mainloop
  Mainloop()

  // do we want posts from a newsgroup?
  AllowNewsgroup(group string) bool

  // trigger a manual regen of indexes for a root post
  Regen(msg ArticleEntry)
  
}
