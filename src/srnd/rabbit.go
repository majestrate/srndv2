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
  }
  return
}

func rabbitQueue(queue string, chnl *amqp.Channel) (q amqp.Queue, err error) {
  log.Println("[MQ] Declare Queue:", queue)
  q, err = chnl.QueueDeclare(
    queue, // name
    true, // durable
    false, // delete when unused
    false,  // exclusive
    false, // no-wait
    nil,   // arguments
  )
  return 
}
