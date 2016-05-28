// +build !disable_File

package cache

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

type FileCache struct {
}

func (self *FileCache) ServeCached(w http.ResponseWriter, r *http.Request, key string, handler RecacheHandler) {
	_, err := os.Stat(key)
	if os.IsNotExist(err) {
		modtime := time.Now().UTC()
		ts := modtime.Format(http.TimeFormat)

		w.Header().Set("Last-Modified", ts)

		body := handler()
		self.Cache(key, body)
		fmt.Fprintf(w, body)
		return
	}

	http.ServeFile(w, r, key)
}

func (self *FileCache) DeleteCache(key string) {
	err := os.Remove(key)
	if err != nil {
		log.Println("cannot remove file", key, err)
	}
}

func (self *FileCache) Cache(key string, body string) {
	f, err := os.Create(key)
	defer f.Close()

	if err != nil {
		log.Println("cannot cache", key, err)
		return
	}
	fmt.Fprintf(f, body)
}

func (self *FileCache) Close() {
}

func NewFileCache() CacheInterface {
	cache := new(FileCache)

	return cache
}
