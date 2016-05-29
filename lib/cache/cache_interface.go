package cache

import "net/http"

type RecacheHandler func() string

type CacheInterface interface {
	ServeCached(w http.ResponseWriter, r *http.Request, key string, handler RecacheHandler)
	DeleteCache(key string)
	Cache(key string, body string)
	Close()
}
