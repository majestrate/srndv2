package main

import (
  "fmt"
  "github.com/gographics/imagick/imagick"
  "github.com/majestrate/srndv2/src/srnd"
  "os"
  "log"
  //   _ "net/http/pprof"
  //  "net/http"
  "time"
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
      if daemon.Init() {
        imagick.Initialize()
        daemon.Run()
        imagick.Terminate()
      } else {
        log.Println("Failed to initialize")
      }
    } else if action == "worker" {
      log.Printf("Run %s As Worker...", srnd.Version())
      if srnd.WorkerInit() {
        for {
          log.Println("Start worker...")
          srnd.WorkerRun()
          log.Println("worker exited")
          time.Sleep(1 * time.Second)
        }
      } else {
        log.Println("worker failed to initialize")
      }
    } else {
      log.Println("Invalid action:",action)
    }
  } else {
    fmt.Fprintf(os.Stdout, "Usage: %s [worker|setup|run]\n", os.Args[0])
  }
}
