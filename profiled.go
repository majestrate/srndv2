package main

import (
  "github.com/gographics/imagick/imagick"
  "github.com/majestrate/srndv2/src/srnd"
  "net/http"
  "log"
  _ "net/http/pprof"
)




func main() {
  go func() {
    log.Println(http.ListenAndServe("[::]:6060", nil))
  }()
  var daemon srnd.NNTPDaemon
  log.Println("Starting up SRNd...")
  if daemon.Init() {
    imagick.Initialize()
    daemon.Run()
    imagick.Terminate()
  } else {
    log.Println("Failed to initialize")
  }
}
