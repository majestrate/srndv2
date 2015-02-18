//
// store.go
//

package main

import (
  "log"
  "os"
  "path/filepath"
)
// iterator hook function
// return true on error
type StoreIteratorHook func (fname string) bool

type ArticleStore struct {
  directory string
}

// initialize article store
func (self *ArticleStore) Init() {
  EnsureDir(self.directory)
}

// iterate over the articles in this article store
// call a hookfor each article passing in the messageID
func (self *ArticleStore) IterateAllArticles(hook StoreIteratorHook) error {
  f , err := os.Open(self.directory)
  if err != nil {
    return err
  }
  var names []string
  names, err = f.Readdirnames(-1)
  for idx := range names {
    fname := names[idx]
    if hook(fname) {
      break
    }
  }
  f.Close()
  return nil
}

// create a file for this article
func (self *ArticleStore) CreateFile(messageID string) *os.File {
  fname := self.GetFilename(messageID)
  file, err := os.Create(fname)
  if err != nil {
    log.Fatal("cannot open file", fname)
    return nil
  }
  return file
}

// return true if we have an article
func (self *ArticleStore) HasArticle(messageID string) bool {
  return CheckFile(self.GetFilename(messageID))
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
    log.Fatal("cannot open",fname)
    return nil
  }
  message := new(NNTPMessage)
  success := message.LoadHeaders(file)
  file.Close()
  file, err = os.Open(fname)
  if err == nil && loadBody {
    success = message.LoadBody(file)
  }
  if success {
    return message
  }
  log.Println("failed to load file", fname)
  return nil
}
