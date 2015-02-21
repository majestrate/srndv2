//
// database.go
//
package main

import (
  "database/sql"
	_ "github.com/lib/pq"
)

type Database struct {
  conn *sql.DB
  db_url string
}

func (self *Database) Init() error {
  var err error
  self.conn, err = sql.Open("postgres", self.db_url)
  if err != nil {
    log.Fatal("failed to connect to database", err)
    return err
  }
  return err
}