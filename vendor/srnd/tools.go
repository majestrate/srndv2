//
// tools.go -- srndv2 cli tool functions
//
package srnd

import (
	"log"
	"os"
)

// worker for thumbnailer tool
func rethumb(chnl chan string, store ArticleStore) {
	for {
		fname, has := <-chnl
		if !has {
			return
		}
		thm := store.ThumbnailFilepath(fname)
		if CheckFile(thm) {
			log.Println("remove old thumbnail", thm)
			os.Remove(thm)
		}
		log.Println("generate thumbnail for", fname)
		_ = store.GenerateThumbnail(fname)
	}
}

// run thumbnailer with 4 threads
func ThumbnailTool() {
	conf := ReadConfig()
	if conf == nil {
		log.Println("cannot load config, ReadConfig() returned nil")
		return
	}
	store := createArticleStore(conf.store, nil)
	reThumbnail(4, store)
}

func RegenTool() {
	conf := ReadConfig()
	db_host := conf.database["host"]
	db_port := conf.database["port"]
	db_user := conf.database["user"]
	db_passwd := conf.database["password"]
	db_type := conf.database["type"]
	db_sche := conf.database["schema"]
	db := NewDatabase(db_type, db_sche, db_host, db_port, db_user, db_passwd)
	groups := db.GetAllNewsgroups()
	if groups != nil {
		for _, group := range groups {
			go regenGroup(group, db)
		}
	}
}

func regenGroup(name string, db Database) {
	log.Println("regenerating", name)
}

// run thumbnailer tool with unspecified number of threads
func reThumbnail(threads int, store ArticleStore) {

	chnl := make(chan string)

	for threads > 0 {
		go rethumb(chnl, store)
		threads--
	}

	files, err := store.GetAllAttachments()
	if err == nil {
		for _, fname := range files {
			chnl <- fname
		}
	} else {
		log.Println("failed to read attachment directory", err)
	}
	close(chnl)
	log.Println("Rethumbnailing done")
}

// generate a keypair from the command line
func KeygenTool() {
	pub, sec := newSignKeypair()
	log.Println("public key:", pub)
	log.Println("secret key:", sec)
}
