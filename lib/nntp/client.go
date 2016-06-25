package nntp

import (
	"errors"
	"github.com/majestrate/srndv2/lib/model"
	"github.com/majestrate/srndv2/lib/nntp/message"
)

var ErrArticleNotFound = errors.New("article not found")
var ErrPostRejected = errors.New("post rejected")

// an nntp client
// obtains articles from remote nntp server
type Client interface {
	// obtain article by message id
	// returns an article and nil if obtained
	// returns nil and an error if an error occured while obtaining the article,
	// error is ErrArticleNotFound if the remote server doesn't have that article
	Article(msgid model.MessageID) (*message.Article, error)

	// check if the remote server has an article given its message-id
	// return true and nil if the server has the article
	// return false and nil if the server doesn't have the article
	// returns false and error if an error occured while checking
	Check(msgid model.MessageID) (bool, error)

	// check if the remote server carries a newsgroup
	// return true and nil if the server carries this newsgroup
	// return false and nil if the server doesn't carry this newsgroup
	// returns false and error if an error occured while checking
	NewsgroupExists(group model.Newsgroup) (bool, error)

	// return true and nil if posting is allowed
	// return false and nil if posting is not allowed
	// return false and error if an error occured
	PostingAllowed() (bool, error)

	// post an nntp article to remote server
	// returns nil on success
	// returns error if an error ocurred during post
	// returns ErrPostRejected if the remote server rejected our post
	Post(a *message.Article) error

	// connect to remote server
	// returns nil on success
	// returns error if one occurs during dial or handshake
	Connect(d Dialer) error

	// send quit and disconnects from server
	// blocks until done
	Quit()

	// get client authentication strategy
	// return nil if no authentication strategy is to be used
	Auth() ClientAuth
}
