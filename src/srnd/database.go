//
// database.go
//
package srnd

import (
  "database/sql"
  "fmt"
  "log"
  "time"
	_ "github.com/lib/pq"
)

type Database struct {
  conn *sql.DB
}


// initialize this database's internals
// creates tables as needed
func (self *Database) Init(db_host, db_port, db_user, db_password string) error {
  var err error
  db := fmt.Sprintf("user=%s password=%s host=%s port=%s", db_user, db_password, db_host, db_port)
  self.conn, err = sql.Open("postgres", db)
  if err != nil {
    log.Fatal("failed to connect to database", err)
    return err
  }
  self.createTables()
  return nil
}


// finalize all transactions 
// close database connections
func (self *Database) Close() {
  self.conn.Close()
}


// create all tables
// will panic on fail
func (self *Database) createTables() {
  tables := make(map[string]string)


  // table of active newsgroups
  tables["Newsgroups"] = `(
                            name VARCHAR(255) PRIMARY KEY,
                            last_post INTEGER NOT NULL,
                            restricted BOOLEAN
                          )`

  // table for storing nntp article meta data
  tables["Articles"] = `( 
                          message_id VARCHAR(255) PRIMARY KEY,
                          message_id_hash VARCHAR(40) UNIQUE NOT NULL,
                          message_newsgroup VARCHAR(255),
                          message_ref_id VARCHAR(255),
                          time_obtained INTEGER NOT NULL,
                          FOREIGN KEY(message_newsgroup) REFERENCES Newsgroups(name)
                        )`
  
  for k, v := range(tables) {
    _, err := self.conn.Exec(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s%s", k, v))
    if err != nil {
      log.Fatalf("cannot create table %s, %s", k, err)
    }
  }
}


// check if a newsgroup exists
func (self *Database) HasNewsgroup(group string) bool {
  stmt, err := self.conn.Prepare("SELECT COUNT(name) FROM Newsgroups WHERE name = $1")
  if err != nil {
    log.Println("failed to prepare query to check for newsgroup", group, err)
    return false
  }
  defer stmt.Close()
  var count int64
  stmt.QueryRow(group).Scan(&count)
  return count > 0
}

// check if an article exists
func (self *Database) HasArticle(message_id string) bool {
  stmt, err := self.conn.Prepare("SELECT COUNT(message_id) FROM Articles WHERE message_id = $1")
  if err != nil {
    log.Println("failed to prepare query to check for article", message_id, err)
    return false
  }
  defer stmt.Close()
  var count int64
  stmt.QueryRow(message_id).Scan(&count)
  return count > 0
}

// register a new newsgroup
func (self *Database) RegisterNewsgroup(group string) {
  stmt, err := self.conn.Prepare("INSERT INTO Newsgroups (name, last_post) VALUES($1, $2)")
  if err != nil {
    log.Println("failed to prepare query to register newsgroup", group, err)
    return
  }
  defer stmt.Close()
  _, err = stmt.Exec(group, time.Now().Unix())
  if err != nil {
    log.Println("failed to register newsgroup", err)
  }
}

// register a message with the database
func (self *Database) RegisterArticle(message *NNTPMessage) {
  if ! self.HasNewsgroup(message.Newsgroup) {
    self.RegisterNewsgroup(message.Newsgroup)
  }
  stmt, err := self.conn.Prepare("INSERT INTO Articles (message_id, message_id_hash, message_newsgroup, time_obtained, message_ref_id) VALUES($1, $2, $3, $4, $5)")
  if err != nil {
    log.Println("failed to prepare query to register article", message.MessageID, err)
    return
  }
  defer stmt.Close()
  now := time.Now().Unix()
  _, err = stmt.Exec(message.MessageID, HashMessageID(message.MessageID), message.Newsgroup, now, message.Reference)
  if err != nil {
    log.Println("failed to register article", err)
  }
  stmt, err = self.conn.Prepare("UPDATE Newsgroups SET last_post = $1 WHERE name = $2")
  if err != nil {
    log.Println("cannot prepare query to update newsgroup last post", err)
    return
  }
  _, err = stmt.Exec(now, message.Newsgroup)
  if err != nil {
    log.Println("cannot execute query to update newsgroup last post", err)
  }
}

// get all articles in a newsgroup
// send result down a channel
func (self *Database) GetAllArticlesInGroup(group string, recv chan string) {
  stmt, err := self.conn.Prepare("SELECT message_id FROM Articles WHERE message_newsgroup = $1")
  if err != nil {
    log.Printf("failed to prepare query for getting all articles in %s: %s", group, err)
    return
  }
  defer stmt.Close()
  rows, err := stmt.Query(group)
  if err != nil {
    log.Printf("Failed to execute quert for getting all articles in %s: %s", group, err)
    return
  }
  for rows.Next() {
    var msgid string
    rows.Scan(&msgid)
    recv <- msgid
  }
}

// get all articles 
// send result down a channel
func (self *Database) GetAllArticles(recv chan string) {
  stmt, err := self.conn.Prepare("SELECT message_id FROM Articles")
  if err != nil {
    log.Printf("failed to prepare query for getting all articles: %s", err)
    return
  }
  defer stmt.Close()
  rows, err := stmt.Query()
  if err != nil {
    log.Printf("Failed to execute quert for getting all articles: %s", err)
    return
  }
  for rows.Next() {
    var msgid string
    rows.Scan(&msgid)
    recv <- msgid
  }
}
