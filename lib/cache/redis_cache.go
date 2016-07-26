// +build !disable_redis

package cache

import (
	"bytes"
	log "github.com/Sirupsen/logrus"
	"gopkg.in/redis.v3"
	"io"
	"net/http"
	"time"
)

type RedisCache struct {
	client *redis.Client
}

func (self *RedisCache) Has(key string) bool {
	ts, _ := self.client.Get(key + "::Time").Result()
	return len(ts) != 0
}

func (self *RedisCache) ServeCached(w http.ResponseWriter, r *http.Request, key string, handler RecacheHandler) {
	ts, _ := self.client.Get(key + "::Time").Result()
	var modtime time.Time

	if len(ts) == 0 {
		modtime = time.Now().UTC()
		ts = modtime.Format(http.TimeFormat)
	} else {
		modtime, _ = time.Parse(http.TimeFormat, ts)
	}

	//this is stolen from the Go standard library
	if t, err := time.Parse(http.TimeFormat, r.Header.Get("If-Modified-Since")); err == nil && modtime.Before(t.Add(1*time.Second)) {
		h := w.Header()
		delete(h, "Content-Type")
		delete(h, "Content-Length")
		w.WriteHeader(http.StatusNotModified)
		return
	}

	html, err := self.client.Get(key).Result()

	if err == redis.Nil || len(html) == 0 { //cache miss
		w.Header().Set("Last-Modified", ts)
		pr, pw := io.Pipe()
		mw := io.MultiWriter(w, pw)
		err = handler(mw)
		pw.Close()
		if err == nil {
			go func() {
				self.Cache(key, pr)
				pr.Close()
			}()
		} else {
			pr.Close()
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Last-Modified", ts)
	io.WriteString(w, html)
}

func (self *RedisCache) DeleteCache(key string) {
	self.client.Del(key + "::Time")
}

func (self *RedisCache) Cache(key string, body io.Reader) {
	tx, err := self.client.Watch(key, key+"::Time")
	defer tx.Close()

	if err != nil {
		log.Error("cannot cache", key, err)
		return
	}
	t := time.Now().UTC()
	ts := t.Format(http.TimeFormat)

	var b bytes.Buffer
	_, err = io.Copy(&b, body)
	if err == nil {
		tx.Set(key, b.String(), 0)
		tx.Set(key+"::Time", ts, 0)
	}
	if err == nil {
		_, err = tx.Exec(func() error {
			return nil
		})
	}
	if err != nil {
		log.Error("cannot cache", key, err)
	}
}

func (self *RedisCache) Close() {
	if self.client != nil {
		self.client.Close()
		self.client = nil
	}
}

func NewRedisCache(addr, password string) (CacheInterface, error) {
	cache := new(RedisCache)
	cache.client = redis.NewClient(&redis.Options{
		Addr:        addr,
		Password:    password,
		DB:          0, // use default DB
		PoolTimeout: 10 * time.Second,
		PoolSize:    100,
	})

	_, err := cache.client.Ping().Result() //check for successful connection
	if err != nil {
		return nil, err
	}

	return cache, nil
}
