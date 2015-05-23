//
// store.go
//

package srnd

import (
  "errors"
  "io"
  "log"
  "os"
  "path/filepath"
)

type ArticleStore struct {
  directory string
  temp string
  database Database
}

// initialize article store
func (self *ArticleStore) Init() {
  EnsureDir(self.directory)
  EnsureDir(self.temp)
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
func (self *ArticleStore) CreateFile(messageID string) io.WriteCloser {
  fname := self.GetFilename(messageID)
  file, err := os.Create(fname)
  if err != nil {
    log.Println("cannot open file", fname)
    return nil
  }
  return file
}

// create a temp file for inboud articles
func (self *ArticleStore) CreateTempFile(messageID string) io.WriteCloser {
  fname := self.GetTempFilename(messageID)
  file, err := os.Create(fname)
  if err != nil {
    log.Println("cannot open file", fname)
    return nil
  }
  return file
}

// store article, save it in the storage folder
// don't register 
func (self *ArticleStore) StorePost(post *NNTPMessage) error {
  file := self.CreateFile(post.MessageID)
  if file == nil {
    return errors.New("cannot open file for post "+post.MessageID)
  }
  post.WriteTo(file, "\n")
  file.Close()
  log.Printf("stored post %s", post.MessageID)
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

// get the filename for this article
func (self *ArticleStore) GetTempFilename(messageID string) string {
  return filepath.Join(self.temp, messageID)
}

// loads temp message and deletes old article
func (self *ArticleStore) ReadTempMessage(messageID string) *NNTPMessage {
  fname := self.GetTempFilename(messageID)
  nntp := self.readfile(fname)
  defer DelFile(fname)
  return nntp
}

// read a file give filepath
func (self *ArticleStore) readfile(fname string) *NNTPMessage {
  
  file, err := os.Open(fname)
  if err != nil {
    log.Println("store cannot open file",fname)
    return nil
  }
  message := new(NNTPMessage)
  success := message.Load(file, true)
  file.Close()
  if success {
    return message
  }
  
  log.Println("failed to load file", fname)
  return nil
}

// load an article
// return nil on failure
func (self *ArticleStore) GetMessage(messageID string) *NNTPMessage {
  return self.readfile(self.GetFilename(messageID))
}
