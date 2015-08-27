//
// worker.go -- rabbitmq worker
//
package srnd

import (
  "log"
  "os/exec"
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
          true, // auto ack
          false, // exclusive
          false, // no local
          false, // no wait
          nil, // args
        )
        if err == nil {
          // we can now process messages
          log.Println("[MQ] GO!")
          for m := range msgs {
            line := string(m.Body)
            log.Println("[MQ] line:", line)
            parts := strings.Split(line, " ")
            if len(parts) == 2 {
              // assume full filepath
              infname, outfname := parts[0], parts[1]
              cmd := exec.Command(convert, "-thumbnail", "200", infname, outfname)
              exec_out, exec_err := cmd.CombinedOutput()
              log.Println("[MQ] result:", exec_err, exec_out)
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
