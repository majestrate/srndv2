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
    fname, has := <- chnl
    if ! has {
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

// run thumbnailer tool with unspecified number of threads
func reThumbnail(threads int, store ArticleStore) {

  chnl := make(chan string)

  for threads > 0 {
    go rethumb(chnl, store)
    threads --
  }
  
  files, err := store.GetAllAttachments()
  if err == nil {
    for _, fname := range files  {
      chnl <- fname
    }
  } else {
    log.Println("failed to read attachment directory", err)
  }
  close(chnl)
  log.Println("Rethumbnailing done")
}
