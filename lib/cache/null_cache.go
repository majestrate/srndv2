package cache

import (
	"fmt"
	"net/http"
)

type NullCache struct {
}

func (self *NullCache) ServeCached(w http.ResponseWriter, r *http.Request, key string, handler RecacheHandler) {
	fmt.Fprintf(w, handler())
}

func (self *NullCache) DeleteCache(key string) {
}

func (self *NullCache) Cache(key string, body string) {
}

func (self *NullCache) Close() {
}

func NewNullCache() CacheInterface {
	cache := new(NullCache)
	return cache
}
