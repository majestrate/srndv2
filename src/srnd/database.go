//
// database.go
//
package srnd

import (
  "database/sql"
  "log"
)

type ArticleEntry [2]string

type Database interface {
  Login() string
  CreateTables()
  HasNewsgroup(group string) bool
  HasArticle(message_id string) bool
  RegisterNewsgroup(group string)
  RegisterArticle(article NNTPMessage)
  GetAllArticlesInGroup(group string, send chan string)
  GetAllArticles() []ArticleEntry

  // get the number of articles we have
  ArticleCount() int64
  
  // return true if this thread has any replies
  ThreadHasReplies(root_message_id string) bool

  // get all replies to a thread
  // if last > 0 then get that many of the last replies
  GetThreadReplies(root_message_id string, last int) []string

  // get all attachments for this message
  GetPostAttachments(message_id string) []string

  // return true if this newsgroup has posts
  GroupHasPosts(newsgroup string) bool
  
  // get all active threads on a board
  // send each thread's root's message_id down a channel
  GetGroupThreads(newsgroup string, send chan string)

  // get every message id for root posts that need to be expired in a newsgroup
  // threadcount is the upperbound limit to how many root posts we keep
  GetRootPostsForExpiration(newsgroup string, threadcount int) []string

  // get the number of pages a board has
  GetGroupPageCount(newsgroup string) int64
  
  // get board page number N
  // prefix and frontend are injected
  GetGroupForPage(prefix, frontend,  newsgroup string, pageno, perpage int) BoardModel

  // get the root posts of the last N bumped threads globally, for ukko
  GetLastBumpedThreads(newsgroup string, threadcount int) []string

  // get the PostModels for replies to a thread
  // prefix is injected into the post models
  GetThreadReplyPostModels(prefix, rootMessageID string, limit int) []PostModel

  // get a post model for a post
  // prefix is injected into the post model
  GetPostModel(prefix, messageID string) PostModel

  // add a public key to the database
  AddModPubkey(pubkey string) error

  // mark that a mod with this pubkey can act on all boards
  MarkModPubkeyGlobal(pubkey string) error
  
  // revoke mod with this pubkey the privilege of being able to act on all boards
  UnMarkModPubkeyGlobal(pubkey string) error

  // check if this mod pubkey can moderate at a global level
  CheckModPubkeyGlobal(pubkey string) bool
  
  // check if a mod with this pubkey has permission to moderate at all
  CheckModPubkey(pubkey string) bool

  // check if a mod with this pubkey can moderate on the given newsgroup
  CheckModPubkeyCanModGroup(pubkey, newsgroup string) bool
  
  // ban an article
  BanArticle(messageID, reason string) error

  // check if an article is banned or not
  CheckArticleBanned(messageID string) (bool, error)

  // Get ip address given the encrypted version
  // return emtpy string if we don't have it
  GetIPAddress(encAddr string) (string, error)

  // return the encrypted version of an IPAddress
  // if it's not already there insert it into the database
  GetEncAddress(addr string) (string, error)

  // delete an article from the database
  DeleteArticle(msg_id string) error
  
  // underlying database connection
  Conn() *sql.DB

  // get every newsgroup we know of
  GetAllNewsgroups() []string
}

func NewDatabase(db_type, schema, host, port, user, password string) Database  {
  if db_type == "postgres" {
    if schema == "srnd" {
      return NewPostgresDatabase(host, port, user, password)
    }
  }
  log.Fatalf("invalid database type: %s/%s" , db_type, schema)
  return nil
}
