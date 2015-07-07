//
// postgres db backend
//
package srnd

import (
  "database/sql"
  "fmt"
  "log"
  "sort"
  "strings"
  "time"
  _ "github.com/lib/pq"
)

type PostgresDatabase struct {
  conn *sql.DB
  db_str string
}

func NewPostgresDatabase(host, port, user, password string) Database {
  var db PostgresDatabase
  db.db_str = fmt.Sprintf("user=%s password=%s host=%s port=%s client_encoding=UTF-8", user, password, host, port)
  db.conn = db.Conn()
  return db
}

func (self PostgresDatabase) Login() string {
  return self.db_str
}


// finalize all transactions 
// close database connections
func (self PostgresDatabase) Close() {
  self.Conn().Close()
}

func (self PostgresDatabase) Conn() *sql.DB {
  if self.conn == nil {
    log.Println("connecting to database")
    var err error
    self.conn, err = sql.Open("postgres", self.Login())
    if err != nil {
      log.Fatalf("cannot open connection to db: %s", err)
    }
  }
  return self.conn
}

// create all tables
// will panic on fail
func (self PostgresDatabase) CreateTables() {
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

  // table for storing nntp article post content
  tables["ArticlePosts"] = `(
                              newsgroup VARCHAR(255),
                              message_id VARCHAR(255),
                              ref_id VARCHAR(255),
                              name TEXT NOT NULL,
                              subject TEXT NOT NULL,
                              path TEXT NOT NULL,
                              time_posted INTEGER NOT NULL,
                              message TEXT NOT NULL
                            )`

  // table for storing nntp article attachment info
  tables["ArticleAttachments"] = `(
                                    message_id VARCHAR(255),
                                    sha_hash VARCHAR(128) NOT NULL,
                                    filename TEXT NOT NULL,
                                    filepath TEXT NOT NULL
                                  )`
  
  for k, v := range(tables) {
    _, err := self.Conn().Exec(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s%s", k, v))
    if err != nil {
      log.Fatalf("cannot create table %s, %s, login was '%s'", k, err,self.db_str)
    }
  }
}

func (self PostgresDatabase) GetRootPostsForExpiration(newsgroup string, threadcount int) []string {

  //TODO, do this all in 1 query with no bullshit after logic
  
  // root post -> last bump
  threads := make(map[string]int64)
  var rows *sql.Rows
  // get all posts for this newsgroup sorted by oldest post first
  stmt, err := self.Conn().Prepare("SELECT message_id, time_posted, subject, ref_id FROM ArticlePosts WHERE newsgroup = $1 ORDER BY time_posted ASC")
  if err != nil {
    log.Println("failed to prepare query for post expiration step 1", err)
    return nil
  }
  defer stmt.Close()
  rows, err = stmt.Query(newsgroup)
  if err != nil {
    log.Println("failed to execute query for post expiration step 1", err)
    return nil
  }
  // get results
  for rows.Next() {
    var msgid, subject, ref string
    var posted int64
    rows.Scan(&msgid, &posted, &subject, &ref)
    // is this a root post ?
    if len(ref) == 0 {
      // ya
      // record it bumped
      threads[msgid] = posted
      continue
    }
    // check for sage
    subject = strings.ToLower(subject)
    if strings.HasPrefix(subject, "sage ") || subject == "sage" {
      // this is a sage
      // we won't add it to the bump stuff
      continue
    }
    // bump the thread if the root post is there
    bump, ok := threads[ref]
    if ok {
      // bump it if it needs to
      if bump < posted { 
        threads[ref] = posted
      }
    }
  }

  // make map such that: last bumped -> root post
  threads_out := make(map[int64]string)
  var bumps int64Sorter
  for root, bump := range threads {
    threads_out[bump] = root
    bumps = append(bumps, bump)
  }
  //sort by oldest first
  sort.Sort(bumps)
  var roots []string
  // add the oldest thread to the list of expired roots until we have enough threads left
  for len(bumps) >= threadcount {
    roots = append(roots, threads_out[bumps[0]])
    bumps = bumps[1:]
  }
  // return the list of expired roots
  return roots
}

func (self PostgresDatabase) GetAllNewsgroups() []string {
  var rows *sql.Rows
  var err error
  stmt, err := self.Conn().Prepare("SELECT name FROM Newsgroups")
  if err == nil {
    defer stmt.Close()
    rows, err = stmt.Query() 
  }
  if err != nil {
    log.Println("failed to get all newsgroups", err)
    return nil
  }
  
  var groups []string
  
  if rows != nil {
    for rows.Next() {
      var group string
      rows.Scan(&group)
      groups = append(groups, group)
    }
  }
  return groups
}

func (self PostgresDatabase) GetThreadReplies(rootpost string, limit int) []string {
  var rows *sql.Rows
  var err error
  if limit > 0 {
    stmt, err := self.Conn().Prepare("SELECT message_id FROM ArticlePosts WHERE message_id IN ( SELECT message_id FROM ArticlesPosts WHERE ref_id = $1 ORDER BY time_posted DESC LIMIT $2 ) ORDER BY time_posted ASC")
    if err == nil {
      defer stmt.Close()
      rows, err = stmt.Query(rootpost, limit)
    }
  } else {
    stmt, err := self.Conn().Prepare("SELECT message_id FROM ArticlePosts WHERE message_id IN ( SELECT message_id FROM ArticlePosts WHERE ref_id = $1 ) ORDER BY time_posted ASC")
    if err == nil {
      defer stmt.Close()
      rows, err = stmt.Query(rootpost)
    }
    
  }
  if err != nil {
    log.Println("failed to get thread replies", rootpost, err)
    return nil
  }
  
  if rows == nil {
    return nil
  }

  var repls []string
    
  for rows.Next() {
    var msgid string
    rows.Scan(&msgid)
    repls = append(repls, msgid)
  }
  return repls  
}

func (self PostgresDatabase) ThreadHasReplies(rootpost string) bool {
  stmt, err := self.Conn().Prepare("SELECT COUNT(message_id) FROM ArticlePosts WHERE ref_id = $1")
  if err != nil {
    log.Println("failed to prepare query to check for thread replies", rootpost, err)
    return false
  }
  defer stmt.Close()
  var count int64
  stmt.QueryRow(rootpost).Scan(&count)
  return count > 0
}

func (self PostgresDatabase) GetGroupThreads(group string, recv chan string) {
  stmt, err := self.Conn().Prepare("SELECT message_id FROM ArticlePosts WHERE newsgroup = $1 AND ref_id = '' ")
  if err != nil {
    log.Println("failed to prepare query to check for board threads", group, err)
    return
  }
  defer stmt.Close()
  rows, err := stmt.Query(group)
  if err != nil {
    log.Println("failed to execute query to check for board threads", group, err)
  }
  defer rows.Close()
  for rows.Next() {
    var msgid string
    rows.Scan(&msgid)
    recv <- msgid
  }
}

func (self PostgresDatabase) GetLastBumpedThreads(threads int) []string {
  // TODO: detect sage
  stmt, err := self.Conn().Prepare("SELECT message_id, ref_id, time_posted FROM ArticlePosts ORDER BY time_posted DESC")
  if err != nil {
    log.Println("failed to prepare query for get last bumped", err)
    return nil
  }
  defer stmt.Close()
  rows, err := stmt.Query()
  if err != nil {
    log.Println("failed to execute query for get last bumped", err)
  }
  defer rows.Close()

  var roots []string
  for rows.Next() {
    var msgid, refid string
    var posted int64
    rows.Scan(&msgid, &refid, &posted)
    if refid != "" {
      msgid = refid
    }
    put := true
    if len(roots) > 0 {
      for _, root := range roots {
        if root == msgid {
          put = false
          break
        }
      }
    }
    if put {
      roots = append(roots, msgid)
    }
    if len(roots) == threads {
      break
    }
  }
  return roots
}

func (self PostgresDatabase) GroupHasPosts(group string) bool {
  stmt, err := self.Conn().Prepare("SELECT COUNT(message_id) FROM ArticlePosts WHERE newsgroup = $1")
  if err != nil {
    log.Println("failed to prepare query to check for newsgroup posts", group, err)
    return false
  }
  defer stmt.Close()
  var count int64
  stmt.QueryRow(group).Scan(&count)
  return count > 0
}


// check if a newsgroup exists
func (self PostgresDatabase) HasNewsgroup(group string) bool {
  stmt, err := self.Conn().Prepare("SELECT COUNT(name) FROM Newsgroups WHERE name = $1")
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
func (self PostgresDatabase) HasArticle(message_id string) bool {
  stmt, err := self.Conn().Prepare("SELECT COUNT(message_id) FROM Articles WHERE message_id = $1")
  if err != nil {
    log.Println("failed to prepare query to check for article", message_id, err)
    return false
  }
  defer stmt.Close()
  var count int64
  stmt.QueryRow(message_id).Scan(&count)
  return count > 0
}

// check if an article exists
func (self PostgresDatabase) ArticleCount() int64 {
  stmt, err := self.Conn().Prepare("SELECT COUNT(message_id) FROM ArticlePosts")
  if err != nil {
    log.Println("failed to prepare query to get article count", err)
    return -1
  }
  defer stmt.Close()
  var count int64
  stmt.QueryRow().Scan(&count)
  return count 
}

// register a new newsgroup
func (self PostgresDatabase) RegisterNewsgroup(group string) {
  stmt, err := self.Conn().Prepare("INSERT INTO Newsgroups (name, last_post) VALUES($1, $2)")
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

func (self PostgresDatabase) GetPostAttachments(messageID string) []string {
  var atts []string
  stmt, err := self.Conn().Prepare("SELECT filepath FROM ArticleAttachments WHERE message_id = $1")
  if err != nil {
    log.Println("failed to prepare query to get attachments for ", messageID, err)
    return atts
  }
  defer stmt.Close()
  rows, err := stmt.Query(messageID)
  if err != nil {
    log.Println("failed to execute query to get attachments for ", messageID, err)
  }
  defer rows.Close()
  for rows.Next() {
    var val string
    rows.Scan(&val)
    atts = append(atts, val)
  }
  return atts
}

// register a message with the database
func (self PostgresDatabase) RegisterArticle(message *NNTPMessage) {
  if ! self.HasNewsgroup(message.Newsgroup) {
    self.RegisterNewsgroup(message.Newsgroup)
  }
  // insert article metadata
  stmt, err := self.Conn().Prepare("INSERT INTO Articles (message_id, message_id_hash, message_newsgroup, time_obtained, message_ref_id) VALUES($1, $2, $3, $4, $5)")
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
  // update newsgroup
  stmt, err = self.Conn().Prepare("UPDATE Newsgroups SET last_post = $1 WHERE name = $2")
  if err != nil {
    log.Println("cannot prepare query to update newsgroup last post", err)
    return
  }
  defer stmt.Close()
  _, err = stmt.Exec(now, message.Newsgroup)
  if err != nil {
    log.Println("cannot execute query to update newsgroup last post", err)
    return
  }
  // insert article post
  stmt, err = self.Conn().Prepare("INSERT INTO ArticlePosts(newsgroup, message_id, ref_id, name, subject, path, time_posted, message) VALUES($1, $2, $3, $4, $5, $6, $7, $8)")
  if err != nil {
    log.Println("cannot prepare query to insert article post", err)
    return
  }
  defer stmt.Close()
  _, err = stmt.Exec(message.Newsgroup, message.MessageID, message.Reference, message.Name, message.Subject, message.Path, message.Posted, message.Message)
  if err != nil {
    log.Println("cannot insert article post", err)
    return
  }
  // register all attachments
  for _, att := range message.Attachments {
    stmt, err = self.Conn().Prepare("INSERT INTO ArticleAttachments(message_id, sha_hash, filename, filepath) VALUES($1, $2, $3, $4)")
    if err != nil {
      log.Println("failed to prepare query to register attachment", err)
    }
    defer stmt.Close()
    _, err = stmt.Exec(message.MessageID, att.Hash(), att.Name, att.Filename())
    if err != nil {
      log.Println("failed to execute query to register attachment", err)
    }
  }
}

// get all articles in a newsgroup
// send result down a channel
func (self PostgresDatabase) GetAllArticlesInGroup(group string, recv chan string) {
  stmt, err := self.Conn().Prepare("SELECT message_id FROM ArticlePosts WHERE newsgroup = $1")
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
func (self PostgresDatabase) GetAllArticles() []ArticleEntry {
  stmt, err := self.Conn().Prepare("SELECT message_id, newsgroup FROM ArticlePosts")
  if err != nil {
    log.Printf("failed to prepare query for getting all articles: %s", err)
    return nil
  }
  defer stmt.Close()
  rows, err := stmt.Query()
  if err != nil {
    log.Printf("Failed to execute quert for getting all articles: %s", err)
    return nil
  }
  var articles []ArticleEntry
  for rows.Next() {
    var entry ArticleEntry
    rows.Scan(&entry[0], &entry[1])
    articles = append(articles, entry)
  }
  return articles
}
