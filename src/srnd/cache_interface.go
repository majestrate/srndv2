package srnd

import (
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
func NewCache(config map[string]string, db Database, store ArticleStore) CacheInterface {
	prefix := config["prefix"]
	webroot := config["webroot"]
	threads := mapGetInt(config, "regen_threads", 1)
	name := config["name"]
	attachments := mapGetInt(config, "allow_files", 1) == 1

	return NewFileCache(prefix, webroot, name, threads, attachments, db, store)
}
