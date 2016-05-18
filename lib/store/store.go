package store

import (
	"io"
)

// storage for nntp articles and attachments
type Storage interface {
	// store an attachment that we read from an io.Reader
	// filename is used to hint to store what extension to store it as
	// returns absolute filepath where attachment was stored and nil on success
	// returns emtpy string and error if an error ocurred while storing
	StoreAttachment(r io.Reader, filename string) (string, error)

	// ensure the underlying storage backend is created
	Ensure() error
}
