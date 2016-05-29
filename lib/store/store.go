package store

import (
	"errors"
	"io"
)

var ErrNoSuchArticle = errors.New("no such article")

// storage for nntp articles and attachments
type Storage interface {
	// store an attachment that we read from an io.Reader
	// filename is used to hint to store what extension to store it as
	// returns absolute filepath where attachment was stored and nil on success
	// returns emtpy string and error if an error ocurred while storing
	StoreAttachment(r io.Reader, filename string) (string, error)

	// store an article that we read from an io.Reader
	// message id is used to hint where the article is stored
	// returns absolute filepath to where the article was stored and nil on success
	// returns empty string and error if an error ocurred while storing
	StoreArticle(r io.Reader, msgid string) (string, error)

	// return nil if the article with the given message id exists in this storage
	// return ErrNoSuchArticle if it does not exist or an error if another error occured while checking
	HasArticle(msgid string) error

	// delete article from underlying storage
	DeleteArticle(msgid string) error

	// open article for reading
	OpenArticle(msgid string) (io.ReadCloser, error)

	// ensure the underlying storage backend is created
	Ensure() error
}
