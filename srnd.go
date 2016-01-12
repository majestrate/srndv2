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
        } else if tool == "keygen" {
          srnd.KeygenTool()
        } else if tool == "nntp" {
          if len(os.Args) >= 5 {
            var daemon srnd.NNTPDaemon
            daemon.Setup()
            db := daemon.GetDB()
            action := os.Args[3]
            if action == "del-login" {
              username := os.Args[4]
              exists, err := db.CheckNNTPUserExists(username)
              if exists {
                err = db.RemoveNNTPLogin(username)
              } else if err == nil {
                fmt.Fprintf(os.Stdout, "no such user: %s", username)
              } else {
                fmt.Fprintf(os.Stdout, "error while deleting user: %s", err.Error())
              }
            } else if action == "add-login" {
              if len(os.Args) == 6 {
                username := os.Args[4]
                passwd := os.Args[5]
                exists, err := db.CheckNNTPUserExists(username)
                if err == nil {
                  if exists {
                    fmt.Fprintf(os.Stdout, "user %s already exists", username)
                  } else {
                    db.AddNNTPLogin(username, passwd)
                  }
                } else {
                  fmt.Fprintf(os.Stdout, "error checking for user: %s", err.Error())
                }
              } else {
                fmt.Fprintf(os.Stdout, "Usage: %s tool nntp add-login username password\n", os.Args[0])
              }
            } else {
              fmt.Fprintf(os.Stdout, "Usage: %s tool nntp [add-login|del-login]\n", os.Args[0])
            }
          } else {
            fmt.Fprintf(os.Stdout, "Usage: %s tool nntp [add-login|del-login]\n", os.Args[0])
          }
        } else {
          fmt.Fprintf(os.Stdout, "Usage: %s tool [rethumb|keygen|nntp]\n", os.Args[0])
        }
      } else {
        fmt.Fprintf(os.Stdout, "Usage: %s tool [rethumb|keygen|nntp]\n", os.Args[0])
      }
    } else {
      log.Println("Invalid action:",action)
    } 
  } else {
    fmt.Fprintf(os.Stdout, "Usage: %s [setup|run|tool]\n", os.Args[0])
  }
}
