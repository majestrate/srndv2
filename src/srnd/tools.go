//
// tools.go -- srndv2 cli tool functions
//
package srnd

import (
  "log"
  "os"
)

// run thumbnailer
// todo: make multithreaded
func ThumbnailTool() {
  conf := ReadConfig()
  if conf == nil {
    log.Fatal("cannot load config, ReadConfig() returned nil")
  }

  store := createArticleStore(conf.store, nil)

  var generated, removed, errors int64 
  var error_files []string
  files, err := store.GetAllAttachments()
  if err == nil {
    for _, fname := range files  {
      thm := store.ThumbnailFilepath(fname)
      if CheckFile(thm) {
        log.Println("remove old thumbnail", thm)
        os.Remove(thm)
        removed ++
      }
      log.Println("generate thumbnail for", fname)
      err = store.GenerateThumbnail(fname)
      if err == nil {
        generated ++
      } else {
        errors ++
        error_files = append(error_files, fname)
      }
    }
  } else {
    log.Fatal("failed to read attachment directory", err)
  }
  log.Println("Rethumbnailing done")
  log.Printf("generated: %d", generated)
  log.Printf("removed:   %d", removed)
  log.Printf("errors:    %d", errors)
  if errors > 0 {
    log.Println("files failed to generate:")
    for _, fname := range error_files {
      log.Println(fname)
    }
  }
}
