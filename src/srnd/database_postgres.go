//
// postgres db backend
//
package srnd

/**
 * TODO:
 *  ~ caching of board settings
 *  ~ caching of encrypted address info
 *  ~ multithreading check
 */

import (
  "database/sql"
  "encoding/hex"
  "errors"
  "fmt"
  "log"
  "time"
  _ "github.com/lib/pq"
)

type PostgresDatabase struct {
  conn *sql.DB
  db_str string
}

func NewPostgresDatabase(host, port, user, password string) Database {
  var db PostgresDatabase
  db.db_str = fmt.Sprintf("user=%s password=%s host=%s port=%s client_encoding='UTF8'", user, password, host, port)
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
    var err error
    self.conn, err = sql.Open("postgres", self.Login())
    if err != nil {
      log.Fatalf("cannot open connection to db: %s", err)
    }
    log.Println("Connection to postgres backend made")
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


  // table for ip and their encryption key
  tables["EncryptedAddrs"] = `(
                                enckey VARCHAR(255) NOT NULL,
                                addr VARCHAR(255) NOT NULL,
                                encaddr VARCHAR(255) NOT NULL
                              )`
  
  // table for articles that have been banned
  tables["BannedArticles"] = `(
                                message_id VARCHAR(255) PRIMARY KEY,
                                time_banned INTEGER NOT NULL,
                                ban_reason TEXT NOT NULL
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

  // table for thread state
  tables["ArticleThreads"] = `(
                                newsgroup VARCHAR(255) NOT NULL,
                                root_message_id VARCHAR(255) NOT NULL,
                                last_bump INTEGER NOT NULL,
                                last_post INTEGER NOT NULL
                              )`
  
  // table for storing nntp article attachment info
  tables["ArticleAttachments"] = `(
                                    message_id VARCHAR(255),
                                    sha_hash VARCHAR(128) NOT NULL,
                                    filename TEXT NOT NULL,
                                    filepath TEXT NOT NULL
                                  )`

  // table for storing current permissions of mod pubkeys
  tables["ModPrivs"] = `(
                          pubkey VARCHAR(255),
                          newsgroup VARCHAR(255),
                          permission VARCHAR(255)
                        )`

  // table for storing moderation events
  tables["ModLogs"] = `(
                         pubkey VARCHAR(255),
                         action VARCHAR(255),
                         target VARCHAR(255),
                         time INTEGER
                       )`

  // ip range bans
  tables["IPBans"] = `(
                        addr cidr NOT NULL,
                        made INTEGER NOT NULL,
                        expires INTEGER NOT NULL
                      )`
  // bans for encrypted addresses that we don't have the ip for
  tables["EncIPBans"] = `(
                           encaddr VARCHAR(255) NOT NULL,
                           made INTEGER NOT NULL,
                           expires INTEGER NOT NULL
                         )`
  
  for k, v := range(tables) {
    _, err := self.Conn().Exec(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s%s", k, v))
    if err != nil {
      log.Fatalf("cannot create table %s, %s, login was '%s'", k, err,self.db_str)
    }
  }
}

func (self PostgresDatabase) AddModPubkey(pubkey string) error {
  if self.CheckModPubkey(pubkey) {
    log.Println("did not add pubkey", pubkey, "already exists")
    return nil
  }
  stmt, err := self.Conn().Prepare("INSERT INTO ModPrivs(pubkey, newsgroup, permission) VALUES ( $1, $2, $3 )")
  if err != nil {
    return err
  }
  defer stmt.Close()
  rows, err := stmt.Query(pubkey, "ctl", "login")
  if rows != nil {
    rows.Close()
  }
  // TODO: modlogs
  return err
}

func (self PostgresDatabase) CheckModPubkeyGlobal(pubkey string) bool {
  stmt, err := self.Conn().Prepare("SELECT COUNT(*) FROM ModPrivs WHERE pubkey = $1 AND newsgroup = $2 AND permission = $3")
  var result int64
  if err == nil {
    defer stmt.Close()
    stmt.QueryRow(pubkey, "overchan", "all").Scan(&result)
  }
  return result > 0
}

func (self PostgresDatabase) CheckModPubkeyCanModGroup(pubkey, newsgroup string) bool {
  stmt, err := self.Conn().Prepare("SELECT COUNT(*) FROM ModPrivs WHERE pubkey = $1 AND newsgroup = $2")
  var result int64
  if err == nil {
    defer stmt.Close()
    stmt.QueryRow(pubkey, newsgroup).Scan(&result)
  }
  return result > 0
}

func (self PostgresDatabase) CountPostsInGroup(newsgroup string, time_frame int64) (result int64) {
  stmt, err := self.Conn().Prepare("SELECT COUNT(*) FROM ArticlePosts WHERE time_posted > $2 AND newsgroup = $1")
  if err == nil {
    defer stmt.Close()
    if time_frame > 0 {
      time_frame = timeNow() - time_frame
    } else if time_frame < 0 {
      time_frame = 0
    }
    stmt.QueryRow(newsgroup, time_frame).Scan(&result)
  } else {
    result = -1
    log.Println("cannot count posts", err)
  }
  return
}

func (self PostgresDatabase) CheckModPubkey(pubkey string) bool {
  stmt, err := self.Conn().Prepare("SELECT COUNT(*) FROM ModPrivs WHERE pubkey = $1 AND newsgroup = $2 AND permission = $3")
  var result int64
  if err == nil {
    defer stmt.Close()
    stmt.QueryRow(pubkey, "ctl", "login").Scan(&result)
  }
  return result > 0
}

func (self PostgresDatabase) BanArticle(messageID, reason string) error {
  stmt, err := self.Conn().Prepare("INSERT INTO BannedArticles(message_id, time_banned, ban_reason) VALUES($1, $2, $3)")
  if err == nil {
    defer stmt.Close()
    _, err = stmt.Exec(messageID, timeNow(), reason)
  }
  return err
}

func (self PostgresDatabase) CheckArticleBanned(messageID string) (bool, error) {
  stmt, err := self.Conn().Prepare("SELECT COUNT(message_id) FROM BannedArticles WHERE message_id = $1")
  if err == nil {
    defer stmt.Close()
    var count int64
    err = stmt.QueryRow(messageID).Scan(&count)
    return count > 0, err
  }
  return false, err
}

func (self PostgresDatabase) GetEncAddress(addr string) (string, error) {
  stmt, err := self.Conn().Prepare("SELECT COUNT(addr) FROM EncryptedAddrs WHERE addr = $1")
  if err == nil {
    defer stmt.Close()
    var count int64
    err = stmt.QueryRow(addr).Scan(&count)
    if err == nil {
      if count == 0 {
        // needs to be inserted
        stmt, err = self.Conn().Prepare("INSERT INTO EncryptedAddrs(enckey, encaddr, addr) VALUES($1, $2, $3)")
        var encaddr, key string
        if err == nil {
          defer stmt.Close()
          key, encaddr = newAddrEnc(addr)
          if len(encaddr) == 0 {
            err = errors.New("failed to generate new encryption key")
          }
          if err == nil {
            _, err = stmt.Exec(key, encaddr, addr)
          }
        }
        return encaddr, err
      } else {
        stmt, err = self.Conn().Prepare("SELECT encAddr FROM EncryptedAddrs WHERE addr = $1 LIMIT 1")
        if err == nil {
          defer stmt.Close()
          var encaddr string
          err = stmt.QueryRow(addr).Scan(&encaddr)
          return encaddr, err
        }
      }
    }
  }
  return "", err
}

func (self PostgresDatabase) CheckIPBanned(addr string) (banned bool, err error) {
  stmt, err := self.Conn().Prepare("SELECT COUNT(*) FROM IPBans WHERE addr >>= $1 ")
  if err == nil {
    defer stmt.Close()
    var amount int64
    err = stmt.QueryRow(addr).Scan(&amount)
    banned = amount > 0
  }
  return banned, err
}

func (self PostgresDatabase) GetIPAddress(encaddr string) (string, error) {
  stmt, err := self.Conn().Prepare("SELECT COUNT(encAddr) FROM EncryptedAddrs WHERE encAddr = $1")
  if err == nil {
    defer stmt.Close()
    var count int64
    err = stmt.QueryRow(encaddr).Scan(&count)
    if err == nil {
      if count == 0 {
        return "", nil
      } else {
        stmt, err = self.Conn().Prepare("SELECT addr FROM EncryptedAddrs WHERE encAddr = $1 LIMIT 1")
        if err == nil {
          defer stmt.Close()
          var addr string
          err = stmt.QueryRow(encaddr).Scan(&addr)
          return addr, err
        }
      }
    }
  }
  return "", err
}

func (self PostgresDatabase) MarkModPubkeyGlobal(pubkey string) error {
  if self.CheckModPubkeyGlobal(pubkey) {
    // already marked
    log.Println("pubkey already marked as global", pubkey)
    return nil
  }
  stmt, err := self.Conn().Prepare("INSERT INTO ModPrivs(pubkey, newsgroup, permission) VALUES ( $1, $2, $3 )")
  if err != nil {
    return err
  }
  defer stmt.Close()
  _ , err = stmt.Exec(pubkey, "overchan", "all")
  return err
}

func (self PostgresDatabase) UnMarkModPubkeyGlobal(pubkey string) error {
  if self.CheckModPubkeyGlobal(pubkey) {
    // already marked
    
    stmt, err := self.Conn().Prepare("DELETE FROM ModPrivs WHERE pubkey = $1 AND newsgroup = $2 AND permission = $3")
    if err != nil {
      return err
    }
    defer stmt.Close()
    _ , err = stmt.Exec(pubkey, "overchan", "all")
    return err
  }
  return errors.New("public key not marked as global")
}



func (self PostgresDatabase) GetRootPostsForExpiration(newsgroup string, threadcount int) []string {

  var rows *sql.Rows
  stmt, err := self.Conn().Prepare("SELECT root_message_id FROM ArticleThreads WHERE newsgroup = $1 AND root_message_id NOT IN ( SELECT root_message_id FROM ArticleThreads WHERE newsgroup = $1 ORDER BY last_bump DESC LIMIT $2)")
  if err != nil {
    log.Println("failed to prepare query for post expiration step", err)
    return nil
  }
  defer stmt.Close()
  rows, err = stmt.Query(newsgroup, threadcount)
  if err != nil {
    log.Println("failed to execute query for post expiration", err)
    return nil
  }
  var roots []string
  defer rows.Close()
  // get results
  for rows.Next() {
    var root string
    rows.Scan(&root)
    roots = append(roots, root)
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
    defer rows.Close()
    for rows.Next() {
      var group string
      rows.Scan(&group)
      groups = append(groups, group)
    }
  }
  return groups
}

func (self PostgresDatabase) GetGroupPageCount(newsgroup string) int64 {
  stmt, err := self.Conn().Prepare("SELECT COUNT(*) FROM ArticleThreads WHERE newsgroup = $1")
  if err != nil {
    log.Println("failed to prepare query to get board page count", err)
    return -1
  }
  defer stmt.Close()
  var count int64
  stmt.QueryRow(newsgroup).Scan(&count)
  // divide by threads per page
  return ( count / 10 ) + 1
}

// TODO: optimize
func (self PostgresDatabase) GetGroupForPage(prefix, frontend, newsgroup string, pageno, perpage int) BoardModel {
  var threads []ThreadModel

  // TODO: hard coded value
  roots := self.GetLastBumpedThreads(newsgroup, 100)

  pages := self.GetGroupPageCount(newsgroup)
  
  min_thread := pageno * perpage
  max_thread := ( ( pageno + 1 ) * perpage ) 
  
  // for each OP
  for thread_no, root_msg_id := range roots {
    // is this in our range?
    if thread_no < min_thread || thread_no >= max_thread {
      // no
      continue
    }
    var posts []PostModel
    // get op
    op := self.GetPostModel(prefix, root_msg_id)
    if op == nil {
      log.Println("failed to get OP, was nil:", root_msg_id)
      return nil
    }
    posts = append(posts, op)
    // append replies
    if self.ThreadHasReplies(root_msg_id) {
      // TODO: harcoded value
      repls := self.GetThreadReplyPostModels(prefix, root_msg_id, 5)
      if repls == nil {
        log.Println("failed to get replies to", root_msg_id)
        return nil
      }
      posts = append(posts, repls...)
    }
    // add thread to board page
    threads = append(threads, thread{
      prefix: prefix,
      posts: posts,
    })
  }
  
  return boardModel{
    prefix: prefix,
    frontend: frontend,
    board: newsgroup,
    page: pageno,
    pages: int(pages),
    threads: threads,
  }
}

func (self PostgresDatabase) GetPostModel(prefix, messageID string) PostModel {
  stmt, err := self.Conn().Prepare("SELECT newsgroup, message_id, ref_id, name, subject, path, time_posted, message FROM ArticlePosts WHERE message_id = $1")
  if err != nil {
    log.Println("failed to prepare query for geting post model for", messageID, err)
    return nil
  }
  defer stmt.Close()
  model := post{}
  model.prefix = prefix
  row := stmt.QueryRow(messageID)
  row.Scan(&model.board, &model.message_id, &model.parent, &model.name, &model.subject, &model.path, &model.posted, &model.message)
  model.op = len(model.parent) == 0
  if len(model.parent) == 0 {
      model.parent = model.message_id
  }
  model.sage = isSage(model.subject)
  atts := self.GetPostAttachmentModels(prefix, messageID)
  if atts != nil {
    model.attachments = append(model.attachments, atts...)
  }
  return model
}

func (self PostgresDatabase) DeleteThread(msgid string) (err error) {
  stmt, err := self.Conn().Prepare("DELETE FROM ArticleThreads WHERE root_message_id = $1")
  if err == nil {
    defer stmt.Close()
     _ = stmt.QueryRow(msgid)
  }
  return
}

func (self PostgresDatabase) DeleteArticle(msgid string) (err error) {
  
  stmt, err := self.Conn().Prepare("DELETE FROM ArticlePosts WHERE message_id = $1")
  if err == nil {
    defer stmt.Close()
    _ = stmt.QueryRow(msgid)
    // delete attachments too
    stmt, err = self.Conn().Prepare("DELETE FROM ArticleAttachments WHERE message_id = $1")
    if err == nil {
      defer stmt.Close()
      _ = stmt.QueryRow(msgid)
    }
  }
  return
}

func (self PostgresDatabase) GetThreadReplyPostModels(prefix, rootpost string, limit int) []PostModel {
  var rows *sql.Rows
  var err error
  if limit > 0 {
    stmt, err := self.Conn().Prepare("SELECT newsgroup, message_id, ref_id, name, subject, path, time_posted, message FROM ArticlePosts WHERE message_id IN ( SELECT message_id FROM ArticlePosts WHERE ref_id = $1 ORDER BY time_posted DESC LIMIT $2 ) ORDER BY time_posted ASC")
    if err == nil {
      defer stmt.Close()
      rows, err = stmt.Query(rootpost, limit)
    } else {
      log.Println("failed to prepare limited query for", rootpost, err)
    }
  } else {
    stmt, err := self.Conn().Prepare("SELECT newsgroup, message_id, ref_id, name, subject, path, time_posted, message FROM ArticlePosts WHERE message_id IN ( SELECT message_id FROM ArticlePosts WHERE ref_id = $1 ) ORDER BY time_posted ASC")
    if err == nil {
      defer stmt.Close()
      rows, err = stmt.Query(rootpost)
    } else {
      log.Println("failed to prepare unlimited query for", rootpost, err)
    }
  }
  
  if err != nil {
    log.Println("failed to get thread replies", rootpost, err)
    return nil
  }
  
  if rows == nil {
    log.Println("rows is nil")
    return nil
  }
  
  var repls []PostModel
  defer rows.Close()
  for rows.Next() {
    model := post{}
    model.prefix = prefix
    rows.Scan(&model.board, &model.message_id, &model.parent, &model.name, &model.subject, &model.path, &model.posted, &model.message)
    model.op = len(model.parent) == 0
    if len(model.parent) == 0 {
      model.parent = model.message_id
    }
    model.sage = isSage(model.subject)
    atts := self.GetPostAttachmentModels(prefix, model.message_id)
    if atts != nil {
      model.attachments = append(model.attachments, atts...)
    }
    
    repls = append(repls, model)
  }
  return repls  

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
  defer rows.Close()
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
    if stmt != nil {
      stmt.Close()
    }
    return false
  }
  var count int64
  stmt.QueryRow(rootpost).Scan(&count)
  stmt.Close()
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

func (self PostgresDatabase) GetLastBumpedThreads(newsgroup string, threads int) []string {
  var err error
  var rows *sql.Rows
  if len(newsgroup) > 0 { 
    stmt, err := self.Conn().Prepare("SELECT root_message_id FROM ArticleThreads WHERE newsgroup = $1 ORDER BY last_bump DESC LIMIT $2")
    if err == nil {
      defer stmt.Close()
      rows, err = stmt.Query(newsgroup, threads)
    } else {
      log.Println("failed to prepare query for get last bumped", err)
    }
  } else {
    stmt, err := self.Conn().Prepare("SELECT root_message_id FROM ArticleThreads ORDER BY last_bump DESC LIMIT $1")
    if err == nil {
      defer stmt.Close()
      rows, err = stmt.Query(threads)
    } else {
      log.Println("failed to prepare query for get last bumped", err)
    }
  }
  if err != nil {
    log.Println("failed to execute query for get last bumped", err)
  }
  if rows == nil {
    return nil
  }
  defer rows.Close()

  var roots []string
  for rows.Next() {
    var root string
    rows.Scan(&root)
    roots = append(roots, root)
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

// check if an article exists locally
func (self PostgresDatabase) HasArticleLocal(message_id string) bool {
  stmt, err := self.Conn().Prepare("SELECT COUNT(message_id) FROM ArticlePosts WHERE message_id = $1")
  if err != nil {
    log.Println("failed to prepare query to check for local article", message_id, err)
    return false
  }
  defer stmt.Close()
  var count int64
  stmt.QueryRow(message_id).Scan(&count)
  return count > 0
}

// count articles we have
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
    return atts
  }
  defer rows.Close()
  for rows.Next() {
    var val string
    rows.Scan(&val)
    atts = append(atts, val)
  }
  return atts
}


func (self PostgresDatabase) GetPostAttachmentModels(prefix, messageID string) []AttachmentModel {
  var atts []AttachmentModel
  stmt, err := self.Conn().Prepare("SELECT filepath, filename FROM ArticleAttachments WHERE message_id = $1")
  if err != nil {
    log.Println("failed to prepare query to get attachments for ", messageID, err)
    return atts
  }
  defer stmt.Close()
  rows, err := stmt.Query(messageID)
  if err != nil {
    log.Println("failed to execute query to get attachments for ", messageID, err)
    return atts
  }
  defer rows.Close()
  for rows.Next() {
    var fpath, fname string
    rows.Scan(&fpath, &fname)
    atts = append(atts, attachment{
      prefix: prefix,
      thumbnail: prefix+"thm/"+fpath,
      source: prefix+"img/"+fpath,
      filename: fname,
    })
  }
  return atts
}

// register a message with the database
func (self PostgresDatabase) RegisterArticle(message NNTPMessage) {
  
  msgid := message.MessageID()
  group := message.Newsgroup()
  
  if ! self.HasNewsgroup(group) {
    self.RegisterNewsgroup(group)
  }
  if self.HasArticle(msgid) {
    return
  }
  // insert article metadata
  stmt, err := self.Conn().Prepare("INSERT INTO Articles (message_id, message_id_hash, message_newsgroup, time_obtained, message_ref_id) VALUES($1, $2, $3, $4, $5)")
  if err != nil {
    log.Println("failed to prepare query to register article", msgid, err)
    return
  }
  defer stmt.Close()
  now := time.Now().Unix()
  _, err = stmt.Exec(msgid, HashMessageID(msgid), group, now, message.Reference())
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
  _, err = stmt.Exec(now, group)
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
  _, err = stmt.Exec(group, msgid, message.Reference(), message.Name(), message.Subject(), message.Path(), message.Posted(), message.Message())
  if err != nil {
    log.Println("cannot insert article post", err)
    return
  }

  
  // set / update thread state
  if message.OP() {
    // insert new thread for op
    stmt, err = self.Conn().Prepare("INSERT INTO ArticleThreads(root_message_id, last_bump, last_post, newsgroup) VALUES($1, $2, $2, $3)")
    if err != nil {
      log.Println("cannot prepare query to register thread", msgid, err)
      return
    }
    defer stmt.Close()
    _, err = stmt.Exec(message.MessageID(), message.Posted(), group)
    if err != nil {
      log.Println("cannot execute query to register thread", msgid, err)
      return
    }
  } else {
    ref := message.Reference()
    if ! message.Sage() {
      // bump it nigguh
      stmt, err = self.Conn().Prepare("UPDATE ArticleThreads SET last_bump = $2 WHERE root_message_id = $1")
      if err != nil {
        log.Println("failed to prepare query to bump thread", ref, err)
        return
      }
      defer stmt.Close()
      _, err = stmt.Exec(ref, message.Posted())
      if err != nil {
        log.Println("failed to execute query to bump thread", ref, err)
        return
      }
    }
    // update last posted
    stmt, err = self.Conn().Prepare("UPDATE ArticleThreads SET last_post = $2 WHERE root_message_id = $1")
    if err != nil {
      log.Println("failed to prepare query to update post time for", ref, err)
      return
    }
    defer stmt.Close()
    _, err = stmt.Exec(ref, message.Posted())
    if err != nil {
      log.Println("failed to execute query to update post time for", ref, err)
      return
    }
  }
  
  // register all attachments
  atts := message.Attachments()
  if atts == nil {
    // no attachments
    return
  }
  for _, att := range atts {
    stmt, err = self.Conn().Prepare("INSERT INTO ArticleAttachments(message_id, sha_hash, filename, filepath) VALUES($1, $2, $3, $4)")
    if err != nil {
      log.Println("failed to prepare query to register attachment", err)
      continue
    }
    defer stmt.Close()
    _, err = stmt.Exec(msgid, hex.EncodeToString(att.Hash()), att.Filename(), att.Filepath())
    if err != nil {
      log.Println("failed to execute query to register attachment", err)
      continue
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
  defer rows.Close()
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
  defer rows.Close()
  for rows.Next() {
    var entry ArticleEntry
    rows.Scan(&entry[0], &entry[1])
    articles = append(articles, entry)
  }
  return articles
}


func (self PostgresDatabase) GetPagesPerBoard(group string) (int, error) {
  //XXX: hardcoded
  return 10, nil
}

func (self PostgresDatabase) GetThreadsPerPage(group string) (int, error) {
  //XXX: hardcoded
  return 10, nil
}


func (self PostgresDatabase) GetMessageIDByHash(hash string) (article ArticleEntry, err error) {
  stmt, err := self.Conn().Prepare("SELECT message_id, message_newsgroup FROM Articles WHERE message_id_hash = $1 LIMIT 1")
  if err == nil {
    defer stmt.Close()
    err = stmt.QueryRow(hash).Scan(&article[0], &article[1])
  }
  return article, err
}

func (self PostgresDatabase) BanAddr(addr string) (err error) {
  stmt, err := self.Conn().Prepare("INSERT INTO IPBans(addr, made, expires) VALUES($1, $2, $3)")
  if err == nil {
    defer stmt.Close()
    // ban forever :^)
    _, err = stmt.Exec(addr, timeNow(), -1)
  }
  return
}

func (self PostgresDatabase) CheckEncIPBanned(encaddr string) (banned bool, err error) {
  stmt, err := self.Conn().Prepare("SELECT COUNT(*) FROM EncIPBans WHERE encaddr = $1")
  var result int64
  if err == nil {
    defer stmt.Close()
    stmt.QueryRow(encaddr).Scan(result)
    banned = result > 0
  }
  return 
}

func (self PostgresDatabase) BanEncAddr(encaddr string) (err error) {
  stmt, err := self.Conn().Prepare("INSERT INTO EncIPBans(encaddr, made, expires) VALUES($1, $2, $3)")
  if err == nil {
    defer stmt.Close()
    // ban forever :^)
    _, err = stmt.Exec(encaddr, timeNow(), -1)
  }
  return
}

