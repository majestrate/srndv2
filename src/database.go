//
// database.go
//
package main

import (
  "database/sql"
  "fmt"
  "log"
	_ "github.com/lib/pq"
)

type Database struct {
  conn *sql.DB
}

func (self *Database) Init(db_host, db_port, db_user, db_password string) error {
  var err error
  db := fmt.Sprintf("user=%s password=%s host=%s port=%s", db_user, db_password, db_host, db_port)
  self.conn, err = sql.Open("postgres", db)
  if err != nil {
    log.Fatal("failed to connect to database", err)
    return err
  }
  return err
}