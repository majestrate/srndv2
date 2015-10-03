package main

import (
  "fmt"
  "github.com/majestrate/srndv2/src/srnd"
  "os"
  "log"
  //   _ "net/http/pprof"
  //  "net/http"
)



func main() {

  // debugger
  // go func() {
  //   log.Println(http.ListenAndServe("[::]:6060", nil))
  // }()
  
  if len(os.Args) > 1 {
    action := os.Args[1]
    if action == "setup" {
      var daemon srnd.NNTPDaemon
      log.Println("Setting up SRNd base...")
      daemon.Setup()
      log.Println("Setup Done")
    } else if action == "run" {
      log.Printf("Starting up %s...", srnd.Version())
      var daemon srnd.NNTPDaemon
      daemon.Setup().Run()
    } else if action == "tool" {
      if len(os.Args) > 2 {
        tool := os.Args[2]
        if tool == "rethumb" {
          srnd.ThumbnailTool()
        } else if tool == "prune-threads" {
          // TODO: implement pruning old threads
          // srnd.ClearThreadsTool()
        } else {
          fmt.Fprintf(os.Stdout, "Usage: %s tool [rethumb|prune-threads]\n", os.Args[0])
        }
      } else {
        fmt.Fprintf(os.Stdout, "Usage: %s tool [rethumb|prune-threads]\n", os.Args[0])
      }
    } else {
      log.Println("Invalid action:",action)
    } 
  } else {
    fmt.Fprintf(os.Stdout, "Usage: %s [setup|run|tool]\n", os.Args[0])
  }
}
