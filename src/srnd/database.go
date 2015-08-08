//
// database.go
//
package srnd

import (
  "database/sql"
  "log"
)


// a ( MessageID , newsgroup ) tuple
type ArticleEntry [2]string

func (self ArticleEntry) Newsgroup() string {
  return self[1]
}

func (self ArticleEntry) MessageID() string {
  return self[0]
}

type Database interface {
  Login() string
  CreateTables()
  HasNewsgroup(group string) bool
  HasArticle(message_id string) bool
  HasArticleLocal(message_id string) bool
  RegisterNewsgroup(group string)
  RegisterArticle(article NNTPMessage)
  GetAllArticlesInGroup(group string, send chan string)
  GetAllArticles() []ArticleEntry

  // get an article's MessageID given the hash of the MessageID
  // return an article entry or nil when it doesn't exist + and error if it happened
  GetMessageIDByHash(hash string) (ArticleEntry, error)

  // record that a message given a message id was posted signed by this pubkey
  RegisterSigned(message_id, pubkey string) error
  
  // get the number of articles we have
  ArticleCount() int64
  
  // return true if this thread has any replies
  ThreadHasReplies(root_message_id string) bool

  // get the number of posts in a certain newsgroup since N seconds ago
  // if N <= 0 then count all we have now
  CountPostsInGroup(group string, time_frame int64) int64
  
  // get all replies to a thread
  // if last > 0 then get that many of the last replies
  GetThreadReplies(root_message_id string, last int) []string

  // get all attachments for this message
  GetPostAttachments(message_id string) []string

  // get all attachments for this message
  GetPostAttachmentModels(prefix, message_id string) []AttachmentModel
  

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

  // check if an ip is banned from our local
  CheckIPBanned(addr string) (bool, error)

  // check if an encrypted ip is banned from our local
  CheckEncIPBanned(encAddr string) (bool, error)

  // ban an ip address from the local
  BanAddr(addr string) error

  // unban an ip address from the local
  UnbanAddr(addr string) error
  
  // ban an encrypted ip address from the remote
  BanEncAddr(encAddr string) error
  
  // return the encrypted version of an IPAddress
  // if it's not already there insert it into the database
  GetEncAddress(addr string) (string, error)

  // delete an article from the database
  DeleteArticle(msg_id string) error
  
  // detele the existance of a thread from the threads table, does NOT remove replies
  DeleteThread(root_msg_id string) error
  
  // underlying database connection
  Conn() *sql.DB

  // get threads per page for a newsgroup
  GetThreadsPerPage(group string) (int, error)

  // get pages per board for a newsgroup
  GetPagesPerBoard(group string) (int, error)
  
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
