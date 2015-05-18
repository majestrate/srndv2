//
// database.go
//
package srnd

import (
  "database/sql"
  "log"
)

type Database interface {
  Init(host, port, user, password string) error
  CreateTables()
  HasNewsgroup(group string) bool
  HasArticle(message_id string) bool
  RegisterNewsgroup(group string)
  RegisterArticle(article *NNTPMessage)
  GetAllArticlesInGroup(group string, send chan string)
  GetAllArticles(send chan string)
  Conn() *sql.DB
}

func NewDatabase(db_type string) Database  {
  if db_type == "postgres" {
    return new(PostgresDatabase)
  }
  log.Fatalf("invalid database type: %s" , db_type)
  return nil
}
