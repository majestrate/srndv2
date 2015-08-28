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
  
  
  url := conf.worker["url"]
  convert := conf.worker["convert"]
  conn, chnl, err := rabbitConnect(url)
  if err == nil {
    log.Println("[MQ] Declare Queue...")
    q, err := chnl.QueueDeclare(
      "", // name
      false, // durable
      false, // delete when unused
      true,  // exclusive
      false, // no-wait
      nil,   // arguments
    )
    if err == nil {
      log.Println("[MQ] Queue Bind...", q.Name)
      err = chnl.QueueBind(
        q.Name, // name
        "",     // routing key
        rabbit_exchange, // exchange
        false, nil)
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
            log.Println("[MQ] line:", line)
            parts := strings.Split(line, " ")
            action := parts[0]
            if len(parts) < 2 {
              continue
            }
            args := parts[1:]
            if action == "thumbnail" {
              // assume full filepath
              infname, outfname := args[0], args[1]
              cmd := exec.Command(convert, "-thumbnail", "200", infname, outfname)
              exec_out, exec_err := cmd.CombinedOutput()
              log.Println("[MQ] result:", exec_err, exec_out)
            } else if action == "ukko" {
              genUkko(args[0], args[1], database)
            } else if action == "front" {
              genFrontPage(10, args[1], args[2], database)
            } else if action == "board" {
              page, _ := strconv.ParseInt(args[4], 10, 32)
              genBoardPage(args[0], args[1], args[2], args[3], int(page), database)
            } else if action == "thread" {
              genThread(args[0], args[1], args[2], database)
            }
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
}
