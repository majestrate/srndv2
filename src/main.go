package main

import (
  "log"
)


func main() {
  log.Println("starting up SRNd")
  var daemon NNTPDaemon
  if daemon.Init() {
    daemon.Run()
  }
  log.Println("SRNd done")
}
