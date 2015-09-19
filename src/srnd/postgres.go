//
// postgres db backend
//
package srnd

/**
 * TODO:
 *  ~ caching of board settings
 *  ~ caching of encrypted address info
 *  ~ multithreading check
 *  ~ checking for duplicate articles
 */

import (
  "database/sql"
  "encoding/hex"
  "errors"
  "fmt"
  "log"
  _ "github.com/lib/pq"
)

type PostgresDatabase struct {
  conn *sql.DB
  db_str string
}

func NewPostgresDatabase(host, port, user, password string) Database {
  var db PostgresDatabase
  var err error
  db.db_str = fmt.Sprintf("user=%s password=%s host=%s port=%s client_encoding='UTF8'", user, password, host, port)
  
  log.Println("Connecting to postgres...")
  db.conn, err = sql.Open("postgres", db.db_str)
  if err != nil {
    log.Fatalf("can`not open connection to db: %s", err)
  }

  return db
}

// finalize all transactions 
// close database connections
func (self PostgresDatabase) Close() {
  if self.conn != nil {
    self.conn.Close()
    self.conn = nil
  }
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
  
  // table for storing nntp article posts to pubkey mapping
  tables["ArticleKeys"] = `(
                             message_id VARCHAR(255) NOT NULL,
                             pubkey VARCHAR(255) NOT NULL
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
  var err error
  for k, v := range(tables) {
    // create table
    _, err = self.conn.Exec(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s%s", k, v))
    if err != nil {
      log.Fatalf("cannot create table %s, %s, login was '%s'", k, err,self.db_str)
    }
  }
  // create indexes
  _, err = self.conn.Exec("CREATE INDEX IF NOT EXISTS ON ArticleThreads(root_message_id)")
  _, err = self.conn.Exec("CREATE INDEX IF NOT EXISTS ON ArticleAttachments(message_id)")
  _, err = self.conn.Exec("CREATE INDEX IF NOT EXISTS ON ArticlePosts(message_id)")
  _, err = self.conn.Exec("CREATE INDEX IF NOT EXISTS ON Articles(message_id)")
  _, err = self.conn.Exec("CREATE INDEX IF NOT EXISTS ON Newsgroups(name)")
}

func (self PostgresDatabase) AddModPubkey(pubkey string) error {
  if self.CheckModPubkey(pubkey) {
    log.Println("did not add pubkey", pubkey, "already exists")
    return nil
  }
  _, err := self.conn.Exec("INSERT INTO ModPrivs(pubkey, newsgroup, permission) VALUES ( $1, $2, $3 )", pubkey, "ctl", "login")
  return err
}

func (self PostgresDatabase) GetGroupForMessage(message_id string) (group string, err error) {
  err = self.conn.QueryRow("SELECT newsgroup FROM ArticlePosts WHERE message_id = $1", message_id).Scan(&group)
  return 
}


func(self PostgresDatabase) GetPageForRootMessage(root_message_id string) (group string, page int64, err error) {
  err = self.conn.QueryRow("SELECT newsgroup FROM ArticleThreads WHERE root_message_id = $1", root_message_id).Scan(&group)
  if err == nil {
    perpage, _ := self.GetPagesPerBoard(group)
    err = self.conn.QueryRow("WITH thread(bump) AS (SELECT last_bump FROM ArticleThreads WHERE root_message_id = $1 ) SELECT COUNT(*) FROM ( SELECT last_bump FROM ArticleThreads INNER JOIN thread ON (thread.bump <= ArticleThreads.last_bump AND newsgroup = $2 ) ) AS amount", root_message_id, group).Scan(&page)
    return group, page / int64(perpage), err
  }
  return
}

func (self PostgresDatabase) GetInfoForMessage(msgid string) (root string, newsgroup string, page int64, err error) {
  err = self.conn.QueryRow("SELECT newsgroup, ref_id FROM ArticlePosts WHERE message_id = $1", msgid).Scan(&newsgroup, &root)
  if err == nil {
    if root == "" {
      root = msgid
    }
    perpage, _ := self.GetPagesPerBoard(newsgroup)
    err = self.conn.QueryRow("WITH thread(bump) AS (SELECT last_bump FROM ArticleThreads WHERE root_message_id = $1 ) SELECT COUNT(*) FROM ( SELECT last_bump FROM ArticleThreads INNER JOIN thread ON (thread.bump <= ArticleThreads.last_bump AND newsgroup = $2 ) ) AS amount", root, newsgroup).Scan(&page)
    page = page / int64(perpage)
  }
  return
}

func (self PostgresDatabase) CheckModPubkeyGlobal(pubkey string) bool {
  var result int64
  _ = self.conn.QueryRow("SELECT COUNT(*) FROM ModPrivs WHERE pubkey = $1 AND newsgroup = $2 AND permission = $3", pubkey, "overchan", "all").Scan(&result)
  return result > 0
}

func (self PostgresDatabase) CheckModPubkeyCanModGroup(pubkey, newsgroup string) bool {
  var result int64
  _ = self.conn.QueryRow("SELECT COUNT(*) FROM ModPrivs WHERE pubkey = $1 AND newsgroup = $2", pubkey, newsgroup).Scan(&result)
  return result > 0
}

func (self PostgresDatabase) CountPostsInGroup(newsgroup string, time_frame int64) (result int64) {
  if time_frame > 0 {
    time_frame = timeNow() - time_frame
  } else if time_frame < 0 {
    time_frame = 0
  }
  self.conn.QueryRow("SELECT COUNT(*) FROM ArticlePosts WHERE time_posted > $2 AND newsgroup = $1", newsgroup, time_frame).Scan(&result)
  return
}

func (self PostgresDatabase) CheckModPubkey(pubkey string) bool {
  var result int64
  self.conn.QueryRow("SELECT COUNT(*) FROM ModPrivs WHERE pubkey = $1 AND newsgroup = $2 AND permission = $3", pubkey, "ctl", "login").Scan(&result)
  return result > 0
}

func (self PostgresDatabase) BanArticle(messageID, reason string) error {
  _, err := self.conn.Exec("INSERT INTO BannedArticles(message_id, time_banned, ban_reason) VALUES($1, $2, $3)", messageID, timeNow(), reason)
  return err
}

func (self PostgresDatabase) CheckArticleBanned(messageID string) (result bool, err error) {

  var count int64
  err = self.conn.QueryRow("SELECT COUNT(message_id) FROM BannedArticles WHERE message_id = $1", messageID).Scan(&count)
  result = count > 0
  return 
}

func (self PostgresDatabase) GetEncAddress(addr string) (encaddr string, err error) {
  var count int64
  err = self.conn.QueryRow("SELECT COUNT(addr) FROM EncryptedAddrs WHERE addr = $1", addr).Scan(&count)
  if err == nil {
    if count == 0 {
      // needs to be inserted
      var key string
      key, encaddr = newAddrEnc(addr)
      if len(encaddr) == 0 {
        err = errors.New("failed to generate new encryption key")
      } else {
        _, err = self.conn.Exec("INSERT INTO EncryptedAddrs(enckey, encaddr, addr) VALUES($1, $2, $3)", key, encaddr, addr)
      }
    } else {
      err = self.conn.QueryRow("SELECT encAddr FROM EncryptedAddrs WHERE addr = $1 LIMIT 1", addr).Scan(&encaddr)
    }
  }
  return
}

func (self PostgresDatabase) GetEncKey(encAddr string) (enckey string, err error) {
  err = self.conn.QueryRow("SELECT enckey FROM EncryptedAddrs WHERE encaddr = $1 LIMIT 1", encAddr).Scan(&enckey)
  return
}

func (self PostgresDatabase) CheckIPBanned(addr string) (banned bool, err error) {
  var amount int64
  err = self.conn.QueryRow("SELECT COUNT(*) FROM IPBans WHERE addr >>= $1 ", addr).Scan(&amount)
  banned = amount > 0
  return 
}

func (self PostgresDatabase) GetIPAddress(encaddr string) (addr string, err error) {
  var count int64
  err = self.conn.QueryRow("SELECT COUNT(encAddr) FROM EncryptedAddrs WHERE encAddr = $1", encaddr).Scan(&count)
  if err == nil && count > 0 {
    err = self.conn.QueryRow("SELECT addr FROM EncryptedAddrs WHERE encAddr = $1 LIMIT 1", encaddr).Scan(&addr)
  }
  return
}

func (self PostgresDatabase) MarkModPubkeyGlobal(pubkey string) (err error) {
  if self.CheckModPubkeyGlobal(pubkey) {
    // already marked
    log.Println("pubkey already marked as global", pubkey) 
  } else {
    _, err = self.conn.Exec("INSERT INTO ModPrivs(pubkey, newsgroup, permission) VALUES ( $1, $2, $3 )", pubkey, "overchan", "all")
  }
  return
}

func (self PostgresDatabase) UnMarkModPubkeyGlobal(pubkey string) (err error) {
  if self.CheckModPubkeyGlobal(pubkey) {
    // already marked
    _ , err = self.conn.Exec("DELETE FROM ModPrivs WHERE pubkey = $1 AND newsgroup = $2 AND permission = $3", pubkey, "overchan", "all")
  } else {
    err = errors.New("public key not marked as global")
  }
  return
}

func (self PostgresDatabase) CountThreadReplies(root_message_id string) (repls int64) {
  _ = self.conn.QueryRow("SELECT COUNT(message_id) FROM ArticlePosts WHERE ref_id = $1", root_message_id).Scan(&repls)
  return
}

func (self PostgresDatabase) GetRootPostsForExpiration(newsgroup string, threadcount int) (roots []string) {
  
  rows, err := self.conn.Query("SELECT root_message_id FROM ArticleThreads WHERE newsgroup = $1 AND root_message_id NOT IN ( SELECT root_message_id FROM ArticleThreads WHERE newsgroup = $1 ORDER BY last_bump DESC LIMIT $2)", newsgroup, threadcount)
  if err == nil {
    // get results
    for rows.Next() {
      var root string
      rows.Scan(&root)
      roots = append(roots, root)
      log.Println(root)
    }
    rows.Close()
  } else {
    log.Println("failed to get root posts for expiration", err)
  }
  // return the list of expired roots
  return 
}

func (self PostgresDatabase) GetAllNewsgroups() (groups []string) {

  rows, err := self.conn.Query("SELECT name FROM Newsgroups")
  if err == nil {
    for rows.Next() {
      var group string
      rows.Scan(&group)
      groups = append(groups, group)
    }
    rows.Close()
  }
  return
}

func (self PostgresDatabase) GetGroupPageCount(newsgroup string) int64 {
  var count int64
  err := self.conn.QueryRow("SELECT COUNT(*) FROM ArticleThreads WHERE newsgroup = $1", newsgroup).Scan(&count)
  if err != nil {
    log.Println("failed to count pages in group", newsgroup, err)
  }
  // divide by threads per page
  return ( count / 10 ) + 1
}

// only fetches root posts
// does not update the thread contents
func (self PostgresDatabase) GetGroupForPage(prefix, frontend, newsgroup string, pageno, perpage int) BoardModel {
  var threads []ThreadModel
  pages := self.GetGroupPageCount(newsgroup)
  rows, err := self.conn.Query("WITH roots(root_message_id, last_bump) AS ( SELECT root_message_id, last_bump FROM ArticleThreads WHERE newsgroup = $1 ORDER BY last_bump DESC OFFSET $2 LIMIT $3 ) SELECT p.newsgroup, p.message_id, p.name, p.subject, p.path, p.time_posted, p.message FROM ArticlePosts p INNER JOIN roots ON ( roots.root_message_id = p.message_id ) ORDER BY roots.last_bump DESC", newsgroup, pageno * perpage, perpage)
  if err == nil {
    for rows.Next() {

      p := post{
        prefix: prefix,
      }
      rows.Scan(&p.board, &p.message_id, &p.name, &p.subject, &p.path, &p.posted, &p.message)
      p.parent = p.message_id
      p.op = true
      _ = self.conn.QueryRow("SELECT pubkey FROM ArticleKeys WHERE message_id = $1", p.message_id).Scan(&p.pubkey)
      p.sage = isSage(p.subject)
      atts := self.GetPostAttachmentModels(prefix, p.message_id)
      if atts != nil {
        p.attachments = append(p.attachments, atts...)
      }
      threads = append(threads, thread{
        prefix: prefix,
        posts: []PostModel{p},
        links: []LinkModel{
          linkModel{
            text: newsgroup,
            link: fmt.Sprintf("%s%s-0.html", prefix, newsgroup),
          },
        },
      })
    }
    rows.Close()
  } else {
    log.Println("failed to fetch board model for", newsgroup, "page", pageno, err)
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


func (self PostgresDatabase) GetPostsInGroup(newsgroup string) (models []PostModel, err error) {

  rows, err := self.conn.Query("SELECT newsgroup, message_id, ref_id, name, subject, path, time_posted, message FROM ArticlePosts WHERE newsgroup = $1 ORDER BY time_posted", newsgroup)
  if err == nil {
    for rows.Next() {
      model := post{}
      rows.Scan(&model.board, &model.message_id, &model.parent, &model.name, &model.subject, &model.path, &model.posted, &model.message)
      models = append(models, model)
    }
    rows.Close()
  }
  return
}

func (self PostgresDatabase) GetPostModel(prefix, messageID string) PostModel {
  model := post{}
  err := self.conn.QueryRow("SELECT newsgroup, message_id, ref_id, name, subject, path, time_posted, message FROM ArticlePosts WHERE message_id = $1 LIMIT 1", messageID).Scan(&model.board, &model.message_id, &model.parent, &model.name, &model.subject, &model.path, &model.posted, &model.message)
  if err == nil {
    model.op = len(model.parent) == 0
    if len(model.parent) == 0 {
      model.parent = model.message_id
    }
    model.sage = isSage(model.subject)
    atts := self.GetPostAttachmentModels(prefix, messageID)
    if atts != nil {
      model.attachments = append(model.attachments, atts...)
    }
    // quiet fail
    self.conn.QueryRow("SELECT pubkey FROM ArticleKeys WHERE message_id = $1", messageID).Scan(&model.pubkey)
    return model
  } else {
    log.Println("failed to prepare query for geting post model for", messageID, err)
    return nil
  }
}

func (self PostgresDatabase) DeleteThread(msgid string) (err error) {
  _, err = self.conn.Exec("DELETE FROM ArticleThreads WHERE root_message_id = $1", msgid)
  return
}

func (self PostgresDatabase) DeleteArticle(msgid string) (err error) {
  _, err = self.conn.Exec("DELETE FROM ArticlePosts WHERE message_id = $1", msgid)
  _, err = self.conn.Exec("DELETE FROM ArticleKeys WHERE message_id = $1", msgid)
  _, err = self.conn.Exec("DELETE FROM ArticleAttachments WHERE message_id = $1", msgid)  
  return
}

func (self PostgresDatabase) GetThreadReplyPostModels(prefix, rootpost string, limit int) (repls []PostModel) {
  var rows *sql.Rows
  var err error
  if limit > 0 {
    rows, err = self.conn.Query("SELECT newsgroup, message_id, ref_id, name, subject, path, time_posted, message FROM ArticlePosts WHERE message_id IN ( SELECT message_id FROM ArticlePosts WHERE ref_id = $1 ORDER BY time_posted DESC LIMIT $2 ) ORDER BY time_posted ASC", rootpost, limit)
  } else {
    rows, err = self.conn.Query("SELECT newsgroup, message_id, ref_id, name, subject, path, time_posted, message FROM ArticlePosts WHERE message_id IN ( SELECT message_id FROM ArticlePosts WHERE ref_id = $1 ) ORDER BY time_posted ASC", rootpost)
  }
  
  if err == nil {
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
      // get pubkey if it exists
      // quiet fail
      _ = self.conn.QueryRow("SELECT pubkey FROM ArticleKeys WHERE message_id = $1", model.message_id).Scan(&model.pubkey)
      repls = append(repls, model)
    }
    rows.Close()
  } else {
    log.Println("failed to get thread replies", rootpost, err)
  }

  return  

}

func (self PostgresDatabase) GetThreadReplies(rootpost string, limit int) (repls []string) {
  var rows *sql.Rows
  var err error
  if limit > 0 {
    rows, err = self.conn.Query("SELECT message_id FROM ArticlePosts WHERE message_id IN ( SELECT message_id FROM ArticlesPosts WHERE ref_id = $1 ORDER BY time_posted DESC LIMIT $2 ) ORDER BY time_posted ASC", rootpost, limit)
  } else {
    rows, err = self.conn.Query("SELECT message_id FROM ArticlePosts WHERE message_id IN ( SELECT message_id FROM ArticlePosts WHERE ref_id = $1 ) ORDER BY time_posted ASC", rootpost)
  }
  if err == nil {
    for rows.Next() {
      var msgid string
      rows.Scan(&msgid)
      repls = append(repls, msgid)
    }
    rows.Close()
  } else {
    log.Println("failed to get thread replies", rootpost, err)
  }
  return 
}

func (self PostgresDatabase) ThreadHasReplies(rootpost string) bool {
  var count int64
  err := self.conn.QueryRow("SELECT COUNT(message_id) FROM ArticlePosts WHERE ref_id = $1", rootpost).Scan(&count)
  if err != nil {
    log.Println("failed to count thread replies", err)
  }
  return count > 0
}

func (self PostgresDatabase) GetGroupThreads(group string, recv chan ArticleEntry) {
  rows, err := self.conn.Query("SELECT message_id FROM ArticlePosts WHERE newsgroup = $1 AND ref_id = '' ", group)
  if err == nil {
    for rows.Next() {
      var msgid string
      rows.Scan(&msgid)
      recv <- ArticleEntry{msgid, group}
    }
    rows.Close()
  } else {
    log.Println("failed to get group threads", err)
  }
}

func (self PostgresDatabase) GetLastBumpedThreads(newsgroup string, threads int) (roots []ArticleEntry) {
  var err error
  var rows *sql.Rows
  if len(newsgroup) > 0 { 
    rows, err = self.conn.Query("SELECT root_message_id, newsgroup FROM ArticleThreads WHERE newsgroup = $1 ORDER BY last_bump DESC LIMIT $2", newsgroup, threads)
  } else {
    rows, err = self.conn.Query("SELECT root_message_id, newsgroup FROM ArticleThreads WHERE newsgroup != 'ctl' ORDER BY last_bump DESC LIMIT $1", threads)
  }

  if err == nil {
    for rows.Next() {
      var ent ArticleEntry
      rows.Scan(&ent[0], &ent[1])
      roots = append(roots, ent)
    }
    rows.Close()
  } else {
    log.Println("failed to get last bumped", err)
  }
  return 
}

func (self PostgresDatabase) GroupHasPosts(group string) bool {
  
  var count int64
  err := self.conn.QueryRow("SELECT COUNT(message_id) FROM ArticlePosts WHERE newsgroup = $1", group).Scan(&count)
  if err != nil {
    log.Println("error counting posts in group", group, err)
  }
  return count > 0
}


// check if a newsgroup exists
func (self PostgresDatabase) HasNewsgroup(group string) bool {
  var count int64
  err := self.conn.QueryRow("SELECT COUNT(name) FROM Newsgroups WHERE name = $1", group).Scan(&count)
  if err != nil {
    log.Println("failed to check for newsgroup", group, err)
  }
  return count > 0
}

// check if an article exists
func (self PostgresDatabase) HasArticle(message_id string) bool {
  var count int64
  err := self.conn.QueryRow("SELECT COUNT(message_id) FROM Articles WHERE message_id = $1", message_id).Scan(&count)
  if err != nil {
    log.Println("failed to check for article", message_id, err)
  }
  return count > 0
}

// check if an article exists locally
func (self PostgresDatabase) HasArticleLocal(message_id string) bool {
  var count int64
  err := self.conn.QueryRow("SELECT COUNT(message_id) FROM ArticlePosts WHERE message_id = $1", message_id).Scan(&count)
  if err != nil {
    log.Println("failed to check for local article", message_id, err)
  }
  return count > 0
}

// count articles we have
func (self PostgresDatabase) ArticleCount() (count int64) {

  err := self.conn.QueryRow("SELECT COUNT(message_id) FROM ArticlePosts").Scan(&count)
  if err != nil {
    log.Println("failed to count articles", err)
  }
  return 
}

// register a new newsgroup
func (self PostgresDatabase) RegisterNewsgroup(group string) {
  _, err := self.conn.Exec("INSERT INTO Newsgroups (name, last_post) VALUES($1, $2)", group, timeNow())
  if err != nil {
    log.Println("failed to register newsgroup", group, err)
  }
}

func (self PostgresDatabase) GetPostAttachments(messageID string) (atts []string) {
  rows, err := self.conn.Query("SELECT filepath FROM ArticleAttachments WHERE message_id = $1", messageID)
  if err == nil {
    for rows.Next() {
      var val string
      rows.Scan(&val)
      atts = append(atts, val)
    }
    rows.Close()
  } else {
    log.Println("cannot find attachments for", messageID, err)
  }
  return 
}


func (self PostgresDatabase) GetPostAttachmentModels(prefix, messageID string) (atts []AttachmentModel) {
  rows, err := self.conn.Query("SELECT filepath, filename FROM ArticleAttachments WHERE message_id = $1", messageID)
  if err == nil {
    for rows.Next() {
      var fpath, fname string
      rows.Scan(&fpath, &fname)
      atts = append(atts, attachment{
        prefix: prefix,
        filepath: fpath,
        filename: fname,
      })
    }
    rows.Close()
  } else {
    log.Println("failed to get attachment models for", messageID, err)
  }
  return 
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
  now := timeNow()
  // insert article metadata
  _, err := self.conn.Exec("INSERT INTO Articles (message_id, message_id_hash, message_newsgroup, time_obtained, message_ref_id) VALUES($1, $2, $3, $4, $5)", msgid, HashMessageID(msgid), group, now, message.Reference())
  if err != nil {
    log.Println("failed to insert article metadata", err)
    return
  }
  // update newsgroup
  _, err = self.conn.Exec("UPDATE Newsgroups SET last_post = $1 WHERE name = $2", now, group)
  if err != nil {
    log.Println("failed to update newsgroup last post", err)
    return
  }
  // insert article post
  _, err = self.conn.Exec("INSERT INTO ArticlePosts(newsgroup, message_id, ref_id, name, subject, path, time_posted, message) VALUES($1, $2, $3, $4, $5, $6, $7, $8)", group, msgid, message.Reference(), message.Name(), message.Subject(), message.Path(), message.Posted(), message.Message())
  if err != nil {
    log.Println("cannot insert article post", err)
    return
  }

  
  // set / update thread state
  if message.OP() {
    // insert new thread for op
    _, err = self.conn.Exec("INSERT INTO ArticleThreads(root_message_id, last_bump, last_post, newsgroup) VALUES($1, $2, $2, $3)", message.MessageID(), message.Posted(), group)

    if err != nil {
      log.Println("cannot register thread", msgid, err)
      return
    }
  } else {
    ref := message.Reference()
    if ! message.Sage() {
      // bump it nigguh
      _, err = self.conn.Exec("UPDATE ArticleThreads SET last_bump = $2 WHERE root_message_id = $1", ref, message.Posted())
      if err != nil {
        log.Println("failed to bump thread", ref, err)
        return
      }
    }
    // update last posted
    _, err = self.conn.Exec("UPDATE ArticleThreads SET last_post = $2 WHERE root_message_id = $1", ref, message.Posted())
    if err != nil {
      log.Println("failed to update post time for", ref, err)
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
    _, err = self.conn.Exec("INSERT INTO ArticleAttachments(message_id, sha_hash, filename, filepath) VALUES($1, $2, $3, $4)", msgid, hex.EncodeToString(att.Hash()), att.Filename(), att.Filepath())
    if err != nil {
      log.Println("failed to register attachment", err)
      continue
    }
  }
}

func (self PostgresDatabase) RegisterSigned(message_id , pubkey string) (err error) {
  _, err = self.conn.Exec("INSERT INTO ArticleKeys(message_id, pubkey) VALUES ($1, $2)", message_id, pubkey)
  return 
}

// get all articles in a newsgroup
// send result down a channel
func (self PostgresDatabase) GetAllArticlesInGroup(group string, recv chan ArticleEntry) {
  rows, err := self.conn.Query("SELECT message_id FROM ArticlePosts WHERE newsgroup = $1")
  if err != nil {
    log.Printf("failed to get all articles in %s: %s", group, err)
    return
  }
  for rows.Next() {
    var msgid string
    rows.Scan(&msgid)
    recv <- ArticleEntry{msgid, group}
  }
  rows.Close()
}

// get all articles 
// send result down a channel
func (self PostgresDatabase) GetAllArticles() (articles []ArticleEntry) {
  rows, err := self.conn.Query("SELECT message_id, newsgroup FROM ArticlePosts")
  if err == nil {
    for rows.Next() {
      var entry ArticleEntry
      rows.Scan(&entry[0], &entry[1])
      articles = append(articles, entry)
    }
    rows.Close()
  } else {
    log.Println("failed to get all articles", err)
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
  err = self.conn.QueryRow("SELECT message_id, message_newsgroup FROM Articles WHERE message_id_hash = $1 LIMIT 1", hash).Scan(&article[0], &article[1])
  return
}

func (self PostgresDatabase) BanAddr(addr string) (err error) {
  _, err = self.conn.Exec("INSERT INTO IPBans(addr, made, expires) VALUES($1, $2, $3)", addr, timeNow(), -1)
  return
}


// assumes it is there
func (self PostgresDatabase) UnbanAddr(addr string) (err error) {
  _, err = self.conn.Exec("DELETE FROM IPBans WHERE addr >>= $1", addr)
  return
}

func (self PostgresDatabase) CheckEncIPBanned(encaddr string) (banned bool, err error) {
  var result int64
  err = self.conn.QueryRow("SELECT COUNT(*) FROM EncIPBans WHERE encaddr = $1", encaddr).Scan(&result)
  banned = result > 0
  return 
}

func (self PostgresDatabase) BanEncAddr(encaddr string) (err error) {
  _, err = self.conn.Exec("INSERT INTO EncIPBans(encaddr, made, expires) VALUES($1, $2, $3)", encaddr, timeNow(), -1)
  return
}

func (self PostgresDatabase) GetLastAndFirstForGroup(group string) (last, first int64, err error) {
  err = self.conn.QueryRow("SELECT COUNT(message_id) FROM ArticlePosts WHERE newsgroup = $1", group).Scan(&last)
  if last == 0 {
    last = 0
    first = 1
  } else {
    last += 1
    first = 1
  }
  return
}

func (self PostgresDatabase) GetMessageIDForNNTPID(group string, id int64) (msgid string, err error) {
  if id == 0 {
    id = 1
  }
  err = self.conn.QueryRow("SELECT message_id FROM ArticlePosts WHERE newsgroup = $1 ORDER BY time_posted LIMIT 1 OFFSET $2", group, id - 1).Scan(&msgid)
  return
}

func (self PostgresDatabase) MarkModPubkeyCanModGroup(pubkey, group string) (err error) {
  _, err = self.conn.Exec("INSERT INTO ModPrivs(pubkey, newsgroup) VALUES($1, $2)", pubkey, group)
  return
}

func (self PostgresDatabase) UnMarkModPubkeyCanModGroup(pubkey, group string) (err error) {
  _, err = self.conn.Exec("DELETE FROM ModPrivs WHERE pubkey = $1 AND newsgroup = $2", pubkey, group)
  return
}

func (self PostgresDatabase) IsExpired(root_message_id string) bool {
  return self.HasArticle(root_message_id) && ! self.HasArticleLocal(root_message_id)
}
