// +build !disable_redis

package cache

import (
	"fmt"
	"gopkg.in/redis.v3"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

type RedisCache struct {
	client *redis.Client
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

		body := handler()
		self.Cache(key, body)
		fmt.Fprintf(w, body)
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
	self.client.Del(key)
	self.client.Del(key + "::Time")
}

func (self *RedisCache) Cache(key string, body string) {
	tx, err := self.client.Watch(key, key+"::Time")
	defer tx.Close()

	if err != nil {
		log.Println("cannot cache", key, err)
		return
	}
	t := time.Now().UTC()
	ts := t.Format(http.TimeFormat)

	tx.Set(key, body, 0)
	tx.Set(key+"::Time", ts, 0)

	_, err = tx.Exec(func() error {
		return nil
	})
	if err != nil {
		log.Println("cannot cache", key, err)
	}
}

func (self *RedisCache) Close() {
	if self.client != nil {
		self.client.Close()
		self.client = nil
	}
}

func NewRedisCache(host, port, password string) CacheInterface {
	cache := new(RedisCache)
	log.Println("Connecting to redis...")

	cache.client = redis.NewClient(&redis.Options{
		Addr:        net.JoinHostPort(host, port),
		Password:    password,
		DB:          0, // use default DB
		PoolTimeout: 10 * time.Second,
		PoolSize:    100,
	})

	_, err := cache.client.Ping().Result() //check for successful connection
	if err != nil {
		log.Fatalf("cannot open connection to redis: %s", err)
	}

	return cache
}
