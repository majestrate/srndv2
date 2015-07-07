package main

import (
  "flag"
  "github.com/majestrate/srndv2/src/srnd"
  "log"
)

var action string

func init() {
  flag.StringVar(&action, "action", "", "what action will we run? (setup, run)")
}


func main() {
  var daemon srnd.NNTPDaemon
  flag.Parse()
  if action == "setup" {
    log.Println("Setting up SRNd base...")
    daemon.Setup()
    log.Println("Setup Done")
  } else if action == "run" {
    log.Println("Starting up SRNd...")
    if daemon.Init() {
      daemon.Run()
    } else {
      log.Println("Failed to initialize")
    }
  }
}
