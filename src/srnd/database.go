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
  RegisterArticle(article *NNTPMessage)
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
  // GetGroupPageCount(newsgroup string) int
  
  // get board page number N
  // GetGroupForPage(newsgroup string, pageno, perpage int) BoardModel

  // get the root posts of the last N bumped threads globally, for ukko
  GetLastBumpedThreads(threadcount int) []string
  
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
