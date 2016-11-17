package srnd

import (
	"log"
	"net/http"
)

type CacheInterface interface {
	RegenAll()
	RegenFrontPage()
	RegenOnModEvent(string, string, string, int)
	RegenerateBoard(group string)
	Regen(msg ArticleEntry)

	DeleteThreadMarkup(root_post_id string)
	DeleteBoardMarkup(group string)

	Start()
	Close()

	GetThreadChan() chan ArticleEntry
	GetGroupChan() chan groupRegenRequest
	GetHandler() http.Handler
}

//TODO only pass needed config
func NewCache(cache_type, host, port, user, password string, config map[string]string, db Database, store ArticleStore) CacheInterface {
	prefix := config["prefix"]
	webroot := config["webroot"]
	threads := mapGetInt(config, "regen_threads", 1)
	name := config["name"]
	attachments := mapGetInt(config, "allow_files", 1) == 1

	if cache_type == "file" {
		return NewFileCache(prefix, webroot, name, threads, attachments, db, store)
	}
	if cache_type == "null" {
		return NewNullCache(prefix, webroot, name, attachments, db, store)
	}
	if cache_type == "varnish" {
		url := config["url"]
		bind_addr := config["bind"]
		return NewVarnishCache(url, bind_addr, prefix, webroot, name, attachments, db, store)
	}

	if cache_type == "redis" {
		log.Fatalf("redis cache not supported, use null cache instead")
		return NewRedisCache(prefix, webroot, name, threads, attachments, db, host, port, password)
	}

	log.Fatalf("invalid cache type: %s", cache_type)
	return nil
}
