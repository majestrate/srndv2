package model

import (
	"regexp"
)

var exp_valid_newsgroup = regexp.MustCompilePOSIX(`^[a-zA-Z0-9.]{1,128}$`)

// an nntp newsgroup
type Newsgroup string

// return true if this newsgroup is well formed otherwise false
func (g Newsgroup) Valid() bool {
	return exp_valid_newsgroup.Copy().MatchString(g.String())
}

// get newsgroup as string
func (g Newsgroup) String() string {
	return string(g)
}

// (message-id, newsgroup) tuple
type ArticleEntry [2]string

func (e ArticleEntry) MessageID() MessageID {
	return MessageID(e[0])
}

func (e ArticleEntry) Newsgroup() Newsgroup {
	return Newsgroup(e[1])
}
