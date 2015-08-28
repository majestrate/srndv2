//
// worker.go -- rabbitmq worker
//
package srnd

import (
  "log"
  "os/exec"
  "strconv"
  "strings"
)

func WorkerInit() bool {
  // check for config
  CheckConfig()
  return true
}

// process a line
// do an action given the command
func workerDoLine(line string, conf map[string]string, database Database) {
  parts := strings.Split(line, " ")
  action := parts[0]
  if len(parts) < 2 {
    return
  }
  args := parts[1:]
  if action == "thumbnail" {
    // assume full filepath
    infname, outfname := args[0], args[1]
    cmd := exec.Command(conf["convert"], "-thumbnail", "200", infname, outfname)
    exec_out, exec_err := cmd.CombinedOutput()
    log.Println("[MQ] result:", exec_err, string(exec_out))
  } else if action == "ukko" {
    template.genUkko(args[0], args[1], args[2], database)
  } else if action == "front" {
    template.genFrontPage(10, args[1], args[2], database)
  } else if action == "board-page" {
    page, _ := strconv.ParseInt(args[3], 10, 32)
    template.genBoardPage(args[0], args[1], args[2], int(page), args[4], database)
  } else if action == "board-all" {
    template.genBoard(args[0], args[1], args[2], args[3], database)
  } else if action == "thread" {
    template.genThread(args[0], args[1], args[2], args[3], database)
  }
}

func workerRun(chnl chan string, conf map[string]string, database Database) {
  log.Println("Start Worker")
  for {
    select {
    case line, ok := <- chnl:
      if ok {
        workerDoLine(line, conf, database)
      } else {
        break
      }
    }
  }
  log.Println("Worker Died")
}

// connect to rabbitmq daemon and run
func WorkerRun() {
  // ReadConfig fatals on error
  conf := ReadConfig()
  if conf == nil {
    log.Println("failed to load config")
    return
  }

  db_host := conf.database["host"]
  db_port := conf.database["port"]
  db_user := conf.database["user"]
  db_passwd := conf.database["password"]
  
  log.Println("connecting to database...")
  database := NewDatabase(conf.database["type"], conf.database["schema"], db_host, db_port, db_user, db_passwd)

  // line dispatcher channel
  lineChnl := make(chan string)

  thread_count := conf.worker["threads"]
  threads, _ := strconv.ParseInt(thread_count, 10, 32)
  if threads <= 0 {
    go workerRun(lineChnl, conf.worker, database)
  } else {
    for threads > 0 {
      threads --
      go workerRun(lineChnl, conf.worker, database)
    }
  }
  url := conf.worker["url"]
  conn, chnl, err := rabbitConnect(url)
  if err == nil {
    q, err := rabbitQueue("srndv2", chnl)
    if err == nil {
      err = chnl.Qos(
        1,     // prefetch count
        0,     // prefetch size
        false, // global
      )
      if err == nil {
        log.Println("[MQ] Consume...")
        msgs, err := chnl.Consume(
          q.Name, // name
          "", // consumer
          false, // auto ack
          false, // exclusive
          false, // no local
          false, // no wait
          nil, // args
        )
        if err == nil {
          // we can now process messages
          log.Println("[MQ] GO!")
          for m := range msgs {
            m.Ack(false)
            line := string(m.Body)
            lineChnl <- line
          }
        }
      }
    }
    if err != nil {
      log.Println("[MQ] failed:", err)
    }
  } else {
    log.Println("[MQ] failed connect:", err)
  }
  if chnl != nil {
    chnl.Close()
  }
  if conn != nil {
    conn.Close()
  }
  database.Close()
  close(lineChnl)
}
