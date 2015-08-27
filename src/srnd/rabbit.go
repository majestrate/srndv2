//
// rabbit.go -- rabbitmq interface
//

package srnd

import (
  "github.com/streadway/amqp"
  "log"
)

const rabbit_queue_markup = "srndv2Markup"
const rabbit_queue_thumbnailer = "srndv2Thumbnailer"
const rabbit_exchange = "srndv2"

func rabbitConnect(url string) (conn *amqp.Connection, chnl *amqp.Channel, err error) {
  log.Println("[MQ] Connect to", url)
  conn, err = amqp.Dial(url)
  if err == nil {
    log.Println("[MQ] Create Channel...")
    chnl, err = conn.Channel()
    if err == nil {
      log.Println("[MQ] Declare exchange...")
      err = chnl.ExchangeDeclare(
        rabbit_exchange,    // name
        "fanout",           // type
        true,               // durable
        false,              // auto-deleted
        false,              // internal
        false,              // no-wait
        nil,                // arguments
      )
    }
  }
  return
}
