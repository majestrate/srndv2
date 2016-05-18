package nntp

import (
	"github.com/majestrate/srndv2/lib/nntp/message"
)

const (
	// accepted article
	ARTICLE_ACCEPT = iota
	// reject article, don't send again
	ARTICLE_REJECT
	// defer article, send later
	ARTICLE_DEFER
	// reject + ban
	ARTICLE_BAN
)

type PolicyStatus int

func (s PolicyStatus) String() string {
	switch int(s) {
	case ARTICLE_ACCEPT:
		return "ACCEPTED"
	case ARTICLE_REJECT:
		return "REJECTED"
	case ARTICLE_DEFER:
		return "DEFERRED"
	case ARTICLE_BAN:
		return "BANNED"
	default:
		return "[invalid policy status]"
	}
}

// type defining a policy that determines if we want to accept/reject/defer an
// incoming article
type ArticlePolicy interface {
	// given an article header, return ARTICLE_ACCEPT if we accepted
	CheckHeader(hdr message.Header) int
}
