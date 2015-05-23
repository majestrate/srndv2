//
// database.go
//
package srnd

import (
  "database/sql"
  "log"
)

type Database interface {
  Login() string
  CreateTables()
  HasNewsgroup(group string) bool
  HasArticle(message_id string) bool
  RegisterNewsgroup(group string)
  RegisterArticle(article *NNTPMessage)
  GetAllArticlesInGroup(group string, send chan string)
  GetAllArticles(send chan string)

  // return true if this thread has any replies
  ThreadHasReplies(root_message_id string) bool

  // get all replies to a thread
  // if last > 0 then get that many of the last replies
  GetThreadReplies(root_message_id string, last int) []string

  // return true if this newsgroup has posts
  GroupHasPosts(newsgroup string) bool
  
  // get all active threads on a board
  // send each thread's root's message_id down a channel
  GetGroupThreads(newsgroup string, send chan string)
  
  Conn() *sql.DB
}

func NewDatabase(db_type, schema, host, port, user, password string) Database  {
  if db_type == "postgres" {
    if schema == "srnd" {
      return NewPostgresDatabase(host, port, user, password)
    } else if schema == "infinity-next" {
      // nop
    }
  } else if db_type == "mysql" {
    if schema == "srnd" {
      // nop
    } else if schema == "infinity-next" {
      // nop
    }
  }
  log.Fatalf("invalid database type: %s/%s" , db_type, schema)
  return nil
}
