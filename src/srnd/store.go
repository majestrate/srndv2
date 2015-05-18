//
// store.go
//

package srnd

import (
  "errors"
  "log"
  "os"
  "path/filepath"
)

type ArticleStore struct {
  directory string
  database Database
}

// initialize article store
func (self *ArticleStore) Init() {
  EnsureDir(self.directory)
}

// send every article's message id down a channel for a given newsgroup
func (self *ArticleStore) IterateAllForNewsgroup(newsgroup string, recv chan string) {
  group := filepath.Clean(newsgroup)
  self.database.GetAllArticlesInGroup(group, recv)
}

// send every article's message id down a channel
func (self *ArticleStore) IterateAllArticles(recv chan string) {
  self.database.GetAllArticles(recv)
}

// create a file for this article
func (self *ArticleStore) CreateFile(messageID string) *os.File {
  fname := self.GetFilename(messageID)
  file, err := os.Create(fname)
  if err != nil {
    //log.Fatal("cannot open file", fname)
    return nil
  }
  return file
}

// store article from frontend
// don't register 
func (self *ArticleStore) StorePost(post *NNTPMessage) error {
  file := self.CreateFile(post.MessageID)
  if file == nil {
    return errors.New("cannot open file for post "+post.MessageID)
  }
  post.WriteTo(file, "\n")
  file.Close()
  return nil
}

// return true if we have an article
func (self *ArticleStore) HasArticle(messageID string) bool {
  return self.database.HasArticle(messageID)
}

// get the filename for this article
func (self *ArticleStore) GetFilename(messageID string) string {
  return filepath.Join(self.directory, messageID)
}

// load a file into a message
// pass true to load the body too
// return nil on failure
func (self *ArticleStore) GetMessage(messageID string, loadBody bool) *NNTPMessage {
  fname := self.GetFilename(messageID)
  file, err := os.Open(fname)
  if err != nil {
    //log.Fatal("cannot open",fname)
    return nil
  }
  message := new(NNTPMessage)
  success := message.Load(file, loadBody)
  file.Close()
  if success {
    return message
  }
  log.Println("failed to load file", fname)
  return nil
}
