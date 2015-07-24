package main

import (
  "fmt"
  "github.com/majestrate/srndv2/src/srnd"
  "log"
  "os"
)


func main() {
  var daemon srnd.NNTPDaemon
  if len(os.Args) > 1 {
    action := os.Args[1]
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
    } else {
      log.Println("Invalid action:",action)
    }
  } else {
    fmt.Fprintf(os.Stdout, "Usage: %s [setup|run]\n", os.Args[0])
  }
}
