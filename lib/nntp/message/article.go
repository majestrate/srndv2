package message

import (
	"github.com/majestrate/srndv2/lib/store"
)

// an nntp article
type Article struct {

	// the article's mime header
	Header Header

	// unexported fields ...

}

// get this article's message-id
func (a *Article) MessageID() (msgid string) {
	// try a few variants
	for _, k := range []string{"Message-ID", "Message-Id", "message-id", "Message-id", "MESSAGE-ID", "MessageID"} {
		v := a.Header.Get(k, "")
		if v != "" {
			msgid = v
			break
		}
	}
	return
}

// type that defines a way to read a single article from some underlying io
type ArticleReader interface {
	// read an attachment and store it in an article store
	// blocks until all reads are done
	ReadAndStore(s store.Storage) (*Article, error)
}
