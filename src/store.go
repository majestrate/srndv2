//
// store.go
//

package main

import (
	"log"
	"os"
	"path/filepath"
)

type ArticleStore struct {
	directory string
}

func (self *ArticleStore) OpenFile(messageID string) *os.File {
	fname := self.GetFilename(messageID)
	file, err := os.Create(fname)
	if err != nil {
		log.Fatal("cannot open file", fname)
		return nil
	}
	return file
}

func (self *ArticleStore) HasArticle(messageID string) bool {
	return CheckFile(self.GetFilename(messageID))
}

func (self *ArticleStore) GetFilename(messageID string) string {
	return filepath.Join(self.directory, messageID)
}

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
