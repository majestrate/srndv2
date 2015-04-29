package main

import (
  "log"
  "github.com/majestrate/srndv2/src/srnd"
)


func main() {
  log.Println("starting up SRNd")
  var daemon srnd.NNTPDaemon
  if daemon.Init() {
    daemon.Run()
  }
  log.Println("SRNd done")
}
